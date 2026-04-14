# Module Architecture: cmd

## Scope

The `cmd` module is the executable entrypoint.
It should contain only composition/bootstrap logic, no business rules.

## Main Flow

1. Load config from environment.
2. Initialize logger.
3. Build concrete store (`RedisStore`).
4. Build service (`SessionService`).
5. Build router and start HTTP server.
6. Listen for `SIGINT/SIGTERM` and shutdown gracefully.

## Why It Matters

Keeping bootstrap isolated makes it easier to:

- replace store implementation
- inject mocks in tests
- run different binaries in future (admin API, migration tool)

