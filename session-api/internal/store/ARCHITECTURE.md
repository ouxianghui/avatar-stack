# Module Architecture: internal/store

## Scope

Persistence abstraction and Redis-backed session storage.

## RedisStore

- JSON session snapshots with TTL.
- Health ping for readiness checks.

## Extension Direction

If you need durable audit logs or multi-region replication, introduce a write-behind
queue or an external datastore without changing the `Store` contract used by `service`.
