# Module Architecture: internal/service

## Scope

Implements business rules and orchestration.

This module is the core of control-plane behavior.

## Core Use Cases

- `CreateSession`: initialize state and enqueue worker start.
- `GetSession`: return current snapshot.
- `StopSession`: mark stopping and enqueue worker stop.
- `HandleMediaHook`: apply state transitions from MediaMTX events.
- `Authorize`: enforce role/path/action credential policy.

## State Transition Inputs

1. API command (`create`, `stop`)
2. MediaMTX hook event (`on-ready`, `on-not-ready`, `on-read`, `on-unread`)

## Why Service Layer Exists

- Keep handlers thin.
- Keep storage replaceable.
- Keep business policy testable without HTTP stack.

