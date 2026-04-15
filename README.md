# Avatar Service Stack (MediaMTX + coturn + Go Session API + SoulX Worker)

This stack gives you a production-style topology with protocol decoupling:

- `Client -> WHIP -> MediaMTX (/avatar/{sessionId}/in)`
- `Worker -> RTSP pull (/in) -> process -> RTSP publish (/out)`
- `Client <- WHEP <- MediaMTX (/avatar/{sessionId}/out)`

The default worker mode is `passthrough` so you can validate end-to-end transport first.
Then replace `worker/app/soulx_runner.py` with your SoulX-FlashHead streaming inference pipeline.

The Session API is implemented in Go with:

- graceful shutdown and request timeout middleware
- Redis-backed session state + start/stop queues
- internal MediaMTX auth endpoint (`/internal/mediamtx/auth`)
- internal MediaMTX hook endpoints (`/internal/mediamtx/hooks/{event}`)

## Architecture docs index

- `ARCHITECTURE.md` (system-level overview)
- `session-api/ARCHITECTURE.md`
- `session-api/cmd/ARCHITECTURE.md`
- `session-api/internal/config/ARCHITECTURE.md`
- `session-api/internal/httpapi/ARCHITECTURE.md`
- `session-api/internal/model/ARCHITECTURE.md`
- `session-api/internal/service/ARCHITECTURE.md`
- `session-api/internal/store/ARCHITECTURE.md`
- `mediamtx/ARCHITECTURE.md`
- `coturn/ARCHITECTURE.md`
- `worker/ARCHITECTURE.md`

## 1. Start

```bash
cd avatar-stack
cp .env.example .env
docker compose up -d --build
```

If you change auth fields in `.env` (`WHIP_*` / `WHEP_*`), keep `/avatar-stack/mediamtx/mediamtx.yml` in sync.

## 2. Create a session

```bash
curl -s -X POST http://localhost:8080/sessions \
  -H 'Content-Type: application/json' \
  -d '{"avatar_id":"demo-avatar","worker_mode":"passthrough"}' | jq
```

Response example:

```json
{
  "session_id": "9b5c...",
  "publish": {
    "whip_url": "http://localhost:8889/avatar/9b5c.../in/whip",
    "username": "publisher",
    "password": "publisher-pass"
  },
  "playback": {
    "whep_url": "http://localhost:8889/avatar/9b5c.../out/whep",
    "username": "viewer",
    "password": "viewer-pass"
  }
}
```

### 2b. Passthrough smoke test (publish to `/in`, play `/out`)

After `POST /sessions`, the worker **waits until something is publishing on `/in`** (via MediaMTX control API), then starts passthrough. Publish **after** the session exists.

**1) RTSP publish — try video-only first** (avoids many `400 Bad Request` failures from FFmpeg’s RTSP + mono-AAC SDP against MediaMTX):

```bash
SESSION_ID='<paste session_id from JSON>'

ffmpeg -re -f lavfi -i testsrc=size=640x480:rate=30 \
  -pix_fmt yuv420p -c:v libx264 -preset ultrafast -tune zerolatency \
  -an -f rtsp -rtsp_transport tcp \
  "rtsp://publisher:publisher-pass@127.0.0.1:8554/avatar/${SESSION_ID}/in"
```

**2) RTSP publish — H.264 + AAC (stereo)** if you need audio:

```bash
ffmpeg -re -f lavfi -i testsrc=size=640x480:rate=30 \
  -f lavfi -i sine=frequency=1000:sample_rate=48000 \
  -pix_fmt yuv420p -c:v libx264 -preset ultrafast -tune zerolatency \
  -c:a aac -ar 48000 -ac 2 -b:a 128k \
  -f rtsp -rtsp_transport tcp \
  "rtsp://publisher:publisher-pass@127.0.0.1:8554/avatar/${SESSION_ID}/in"
```

**3) Play `/out`** (viewer credentials):

```bash
ffplay -rtsp_transport tcp \
  "rtsp://viewer:viewer-pass@127.0.0.1:8554/avatar/${SESSION_ID}/out"
```

If publish still returns `400`, turn on `logLevel: debug` in `mediamtx/mediamtx.yml`, retry once, and inspect `docker compose logs mediamtx` for the exact RTSP rejection reason.

**4) WHIP publish (matches MediaMTX docs; needs FFmpeg with `whip` muxer)** — often preferable to RTSP for WebRTC paths:

```bash
ffmpeg -re -f lavfi -i testsrc=size=1280x720:rate=30 \
  -f lavfi -i "sine=frequency=1000:sample_rate=48000" \
  -c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
  -c:a libopus -ar 48000 -ac 2 -b:a 128k \
  -f whip "http://publisher:publisher-pass@localhost:8889/avatar/${SESSION_ID}/in/whip"
```

## 3. Client integration notes (C++)

- Publish to `publish.whip_url` with Basic Auth (`publish.username/password`).
- Play from `playback.whep_url` with Basic Auth (`playback.username/password`).
- For passthrough verification, encode upload stream as `H264 + Opus`.

## 4. Stop session

```bash
curl -s -X DELETE http://localhost:8080/sessions/<session_id> | jq
```

## 5. Replace passthrough with SoulX

Edit `worker/app/soulx_runner.py` and implement:

1. Pull RTSP stream from `--input-rtsp`
2. Extract audio / avatar condition data
3. Run SoulX-FlashHead real-time inference
4. Publish generated stream to `--output-rtsp`

Then create session with:

```bash
curl -s -X POST http://localhost:8080/sessions \
  -H 'Content-Type: application/json' \
  -d '{"avatar_id":"demo-avatar","worker_mode":"soulx"}' | jq
```

## Production hardening checklist

- Set `coturn` `external-ip` and TLS (`5349`) for public network.
- Change all default credentials and move secrets to vault.
- Set `MEDIAMTX_WEBRTC_BASE_URL` to your public HTTPS domain.
- Run at least `2x` worker replicas + session affinity in control plane.
- Add Prometheus/Grafana alerts on media RTT, packet loss, and worker FPS.
- Switch MediaMTX auth from `internal` to `http` and point it to `session-api` if you want per-session policy.
