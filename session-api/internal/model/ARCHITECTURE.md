# Module Architecture: internal/model

## Scope

Defines domain models and DTOs shared across layers.

## Key Types

- `Session`, `SessionStatus`: core domain state.
- `SessionPayload`: API response contract.
- `MediaMTXAuthRequest`: normalized auth callback payload.

## Path Utilities

`NormalizePath` and `ParseSessionPath` convert callback path variants into
canonical path identifiers (`avatar/{sessionID}/live`).

This keeps path parsing logic out of handlers and service logic.
