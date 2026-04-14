# Module Architecture: internal/model

## Scope

Defines domain models and DTOs shared across layers.

## Key Types

- `Session`, `SessionStatus`, `WorkerMode`: core domain state.
- `SessionPayload`: API response contract.
- `StartQueueMessage`, `StopQueueMessage`: worker queue contracts.
- `MediaMTXAuthRequest`: normalized auth callback payload.

## Path Utilities

`NormalizePath` and `ParseSessionPath` convert callback path variants into
canonical path identifiers (`avatar/{sessionID}/{in|out}`).

This keeps path parsing logic out of handlers and service logic.

