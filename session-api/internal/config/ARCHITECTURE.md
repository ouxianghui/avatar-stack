# Module Architecture: internal/config

## Scope

Transforms environment variables into one validated `Config` struct.

## Design Notes

- Centralized parsing avoids duplicated env parsing in multiple modules.
- Duration values are parsed once during startup.
- URL normalization avoids accidental double-slash path joins.
- Internal auth allowlist is pre-built as map for O(1) lookup.

## Failure Policy

Invalid critical config (duration format or missing required base URLs)
fails fast at startup.

