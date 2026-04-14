# Session API Architecture

## 1. Responsibility

`session-api` is the control plane of the avatar session system.

It does not move media packets. It manages:

- session lifecycle
- session metadata and state
- worker start/stop orchestration
- MediaMTX auth callback decisions
- MediaMTX hook event ingestion

## 2. Package Layout

- `cmd/session-api`: process bootstrap and graceful shutdown.
- `internal/config`: environment-driven runtime config.
- `internal/httpapi`: HTTP routing and request/response handling.
- `internal/service`: business logic and state transitions.
- `internal/store`: persistence and queue interface + Redis implementation.
- `internal/model`: shared domain models and DTO definitions.

## 3. Dependency Direction

`httpapi -> service -> store`

`model` is shared by all layers.

`cmd` wires concrete implementations together.

## 4. External Interfaces

- Public API:
  - `POST /sessions`
  - `GET /sessions/{sessionID}`
  - `DELETE /sessions/{sessionID}`
  - `GET /healthz`
  - `GET /readyz`
- Internal API:
  - `POST /internal/mediamtx/auth`
  - `POST /internal/mediamtx/hooks/{event}`

## 5. Persistence Model

Each session is stored as a JSON snapshot in Redis with TTL.

Worker orchestration uses two Redis lists:

- start queue
- stop queue

## 6. Evolution Path

Current version uses static credentials for MediaMTX auth decisions.
Next production step is token/JWT policy per session and per role.

