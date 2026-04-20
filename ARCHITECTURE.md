# Avatar Stack Architecture

## 1. System Goal

This stack implements a **streaming router**:

1. Publisher client sends media via WHIP to MediaMTX on `/avatar/{sessionId}/live`.
2. Viewer clients receive the same path via WHEP.
3. Session API tracks session metadata and ingests MediaMTX hooks.

There is **no in-repo media worker**; transcode or avatar pipelines live outside this repository if needed.

## 2. Module Map

- `mediamtx`: media gateway, protocol edge (WHIP/WHEP/RTSP).
- `coturn`: TURN/STUN for WebRTC NAT traversal.
- `session-api`: control plane (session lifecycle, auth, hooks).

## 3. Runtime Data Flow

```mermaid
flowchart LR
  A["Publisher Client"] -->|WHIP /avatar/{id}/live| B["MediaMTX"]
  D["Viewer Client"] -->|WHEP /avatar/{id}/live| B
  B -->|auth + hooks| E["Session API"]
  F["Redis"] <-->|session state| E
```

## 4. Control Plane vs Data Plane

- Data plane: `publisher/viewer <-> mediamtx`.
- Control plane: `session-api <-> redis`, plus callbacks from `mediamtx`.

## 5. Session State Machine (Current)

- `waiting_input`
- `output_live` (publisher ready; path is playable)
- `stopping`
- `stopped`
- `failed`

Legacy values such as `input_live` or `processing` may still appear when reading older Redis snapshots.

State is stored in Redis as a snapshot, updated by API operations and MediaMTX hooks.

## 6. Read Order (for onboarding)

1. `session-api/ARCHITECTURE.md`
2. `session-api/cmd/ARCHITECTURE.md`
3. `session-api/internal/service/ARCHITECTURE.md`
4. `session-api/internal/httpapi/ARCHITECTURE.md`
5. `mediamtx/ARCHITECTURE.md`
