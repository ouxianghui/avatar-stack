# Module Architecture: mediamtx

## Scope

`mediamtx` is the media gateway and protocol edge.

## Responsibilities

- Accept WHIP ingest on `/avatar/{sessionID}/live`.
- Serve WHEP playback on `/avatar/{sessionID}/live`.
- Expose RTSP on the same path (publisher publish, viewer read per `mediamtx.yml`).
- Optionally call session-api for auth and hooks.

## Current Mode

`authMethod: http` delegates **publish/read** authentication to `session-api`.  
`authHTTPExclude` skips HTTP auth for **api/metrics/pprof** (treat those ports as sensitive).

## Credentials

- WHIP/WHEP use Basic Auth: configurable usernames (`WHIP_USERNAME` / `WHEP_USERNAME`) and **per-session tokens** as passwords (issued by session-api).
