# Module Architecture: internal/service

## Scope

Business rules and orchestration for sessions and MediaMTX callbacks.

## Key Methods

- `CreateSession`: initialize state in Redis.
- `StopSession`: mark stopping.
- `HandleMediaHook`: map MediaMTX events to session fields.
- `Authorize`: validate static credentials for WHIP/WHEP paths.

## Design Notes

Service is intentionally thin: no media protocol handling, only coordination and policy.
