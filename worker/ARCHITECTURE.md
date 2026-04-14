# Module Architecture: worker

## Scope

`worker` is the media processing executor.

## Current Runtime Model

- one supervisor process (`app/worker.py`)
- consumes Redis start/stop queues
- manages one subprocess per session

## Session Execution Modes

- `passthrough`: ffmpeg copies input stream to output stream
- `soulx`: runs `app/soulx_runner.py` placeholder entrypoint

## Responsibilities

- translate queue commands into process lifecycle
- keep per-session process table in memory
- cleanup subprocesses on stop/shutdown

## Boundary

Worker does not decide session policy.
Policy is owned by session-api and expressed through queue commands.

