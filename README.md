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
