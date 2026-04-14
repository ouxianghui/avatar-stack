# Module Architecture: mediamtx

## Scope

`mediamtx` is the media gateway and protocol edge.

## Responsibilities

- Accept WHIP ingest on `/avatar/{sessionID}/in`.
- Serve WHEP playback on `/avatar/{sessionID}/out`.
- Expose RTSP endpoints for internal worker pull/push.
- Optionally call session-api for auth and hooks.

## Current Mode

Current config uses `authMethod: internal` for local simplicity.
Production should move to `authMethod: http` backed by `session-api`.

## Input/Output Contracts

- Publisher credentials can publish `.../in`.
- Viewer credentials can read `.../out`.
- Worker credentials can read `.../in` and publish `.../out`.

