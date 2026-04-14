# Avatar Stack Architecture

## 1. System Goal

This stack implements one core flow:

1. Client publishes audio/video via WHIP.
2. Worker pulls input stream, processes audio/avatar pipeline.
3. Worker publishes output stream.
4. Viewers pull output stream via WHEP.

## 2. Module Map

- `mediamtx`: media gateway, protocol edge (WHIP/WHEP/RTSP).
- `coturn`: TURN/STUN for WebRTC NAT traversal.
- `session-api`: control plane (session lifecycle, auth, hooks, queue orchestration).
- `worker`: media processing worker supervisor and runner.

## 3. Runtime Data Flow

```mermaid
flowchart LR
  A["Publisher Client"] -->|WHIP /avatar/{id}/in| B["MediaMTX"]
  B -->|RTSP pull| C["Worker"]
  C -->|RTSP publish /avatar/{id}/out| B
  D["Viewer Client"] -->|WHEP /avatar/{id}/out| B
  E["Session API"] -->|start/stop queue| C
  B -->|auth + hooks| E
  F["Redis"] <-->|session state + queues| E
```

## 4. Control Plane vs Data Plane

- Data plane: `publisher/viewer <-> mediamtx <-> worker`.
- Control plane: `session-api <-> redis`, plus callback relation from `mediamtx`.

This separation keeps media transport stable while allowing business logic to evolve independently.

## 5. Session State Machine (Current)

- `waiting_input`
- `input_live`
- `processing`
- `output_live`
- `stopping`
- `stopped`
- `failed`

State is stored in Redis as a snapshot, updated by API operations and MediaMTX hooks.

## 6. Read Order (for C++ developer onboarding)

1. `session-api/ARCHITECTURE.md`
2. `session-api/cmd/ARCHITECTURE.md`
3. `session-api/internal/service/ARCHITECTURE.md`
4. `session-api/internal/httpapi/ARCHITECTURE.md`
5. `worker/ARCHITECTURE.md`
6. `mediamtx/ARCHITECTURE.md`

