# Avatar Streaming Router (MediaMTX + coturn + Go Session API)

This stack is a **WebRTC routing edge**: one logical path per session where publishers use WHIP and viewers use WHEP.

- `Client -> WHIP -> MediaMTX (/avatar/{sessionId}/live)`
- `Client <- WHEP <- MediaMTX (/avatar/{sessionId}/live)`

The Session API is implemented in Go with:

- graceful shutdown and request timeout middleware
- Redis-backed session state and **bcrypt-hashed short-lived tokens** (issued once on `POST /sessions`)
- MediaMTX **`authMethod: http`** delegating stream auth to `/internal/mediamtx/auth`
- internal MediaMTX hook endpoints (`/internal/mediamtx/hooks/{event}`)

**Credentials:** `publish.password` and `playback.password` are **random per session** and are **only returned in the create response**. `GET /sessions/{id}` returns empty passwords. Tokens expire with Redis TTL (`MEDIAMTX_TOKEN_TTL`, default `1h`). MediaMTX still exposes API/metrics without HTTP auth (`authHTTPExclude`); **do not publish `:9997` / `:9998` to the public internet** without a reverse proxy or firewall.

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

## 1. Start

```bash
cd avatar-stack
cp .env.example .env
docker compose up -d --build
```

If you change `WHIP_USERNAME` / `WHEP_USERNAME` in `.env`, set the same values for `session-api` (clients must send those usernames with the token as the password).

## 2. Create a session

```bash
curl -s -X POST http://localhost:8080/sessions \
  -H 'Content-Type: application/json' \
  -d '{"avatar_id":"demo-avatar"}' | jq
```

Save `publish.password` and `playback.password` from this response; they are not shown again on `GET /sessions`.

Response shape:

```json
{
  "session_id": "9b5c...",
  "publish": {
    "whip_url": "http://localhost:8889/avatar/9b5c.../live/whip",
    "username": "publisher",
    "password": "<one-time opaque token>"
  },
  "playback": {
    "whep_url": "http://localhost:8889/avatar/9b5c.../live/whep",
    "username": "viewer",
    "password": "<one-time opaque token>"
  }
}
```

### 2b. Smoke test (WHIP then WHEP)

Create the session first, then **keep WHIP publishing** before starting WHEP on the same `session_id`.

**1) RTSP publish (video-only)** — optional alternative to WHIP (use `publish.password` from the create response):

```bash
SESSION_ID='<paste session_id from JSON>'
PUB_PASS='<paste publish.password from JSON>'

ffmpeg -re -f lavfi -i testsrc=size=640x480:rate=30 \
  -pix_fmt yuv420p -c:v libx264 -preset ultrafast -tune zerolatency \
  -an -f rtsp -rtsp_transport tcp \
  "rtsp://publisher:${PUB_PASS}@127.0.0.1:8554/avatar/${SESSION_ID}/live"
```

**2) RTSP play** (use `playback.password`):

```bash
PLAY_PASS='<paste playback.password from JSON>'
ffplay -rtsp_transport tcp \
  "rtsp://viewer:${PLAY_PASS}@127.0.0.1:8554/avatar/${SESSION_ID}/live"
```

**3) WHIP publish** (needs FFmpeg with `whip` muxer):

```bash
PUB_PASS='<paste publish.password from JSON>'
ffmpeg -re -f lavfi -i testsrc=size=1280x720:rate=30 \
  -f lavfi -i "sine=frequency=1000:sample_rate=48000" \
  -c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
  -c:a libopus -ar 48000 -ac 2 -b:a 128k \
  -f whip "http://publisher:${PUB_PASS}@localhost:8889/avatar/${SESSION_ID}/live/whip"
```

HTML helpers live under `tools/` (`whip_push_test.html`, `whep_test.html`).

## 3. Client integration notes (C++)

- Publish to `publish.whip_url` with Basic Auth: username `publish.username`, password **the one-time token** from `publish.password` at create time.
- Play from `playback.whep_url` with Basic Auth: username `playback.username`, password **the one-time token** from `playback.password`.
- Prefer `H264 + Opus` for WebRTC paths.

## 4. Stop session

```bash
curl -s -X DELETE http://localhost:8080/sessions/<session_id> | jq
```

## Production hardening checklist

- Set `coturn` `external-ip` and TLS (`5349`) for public network.
- Change all default credentials and move secrets to vault.
- Set `MEDIAMTX_WEBRTC_BASE_URL` to your public HTTPS domain.
- Add Prometheus/Grafana alerts on media RTT and packet loss.
- Switch MediaMTX auth from `internal` to `http` and point it to `session-api` for per-session policy.
