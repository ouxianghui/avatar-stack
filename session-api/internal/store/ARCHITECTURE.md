# Module Architecture: internal/store

## Scope

Defines persistence abstraction (`Store`) and Redis implementation.

## Responsibilities

- Persist session snapshots.
- Query session snapshots.
- Push worker start/stop queue messages.
- Expose backend liveness (`Ping`).

## Current Redis Data Shape

- Session key: `{SESSION_KEY_PREFIX}{sessionID}`
- Value: JSON-serialized `model.Session`
- TTL: `SESSION_TTL`
- Queue keys: `START_QUEUE`, `STOP_QUEUE`

## Extension Direction

When business grows:

- move long-term records to PostgreSQL
- keep Redis for hot state + queues
- keep Store interface unchanged for service layer

