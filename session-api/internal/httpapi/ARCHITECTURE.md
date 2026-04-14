# Module Architecture: internal/httpapi

## Scope

Provides HTTP transport layer only:

- route registration
- request decoding/validation
- response formatting
- mapping domain errors to HTTP status codes

## Endpoints

- `GET /healthz`, `GET /readyz`
- `POST /sessions`
- `GET /sessions/{sessionID}`
- `DELETE /sessions/{sessionID}`
- `POST /internal/mediamtx/auth`
- `POST /internal/mediamtx/hooks/{event}`

## Design Notes

- Uses chi middleware for request id, real IP, recoverer, timeout.
- MediaMTX payload parser is tolerant to JSON and form payloads.
- Internal auth endpoint supports source-IP allowlist check.

## Non-Responsibilities

- Does not keep state.
- Does not build media URLs directly.
- Does not implement authorization rules itself (delegates to service layer).

