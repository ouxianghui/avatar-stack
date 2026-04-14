import json
import logging
import os
import signal
import subprocess
import time
from dataclasses import dataclass

from redis import Redis

logging.basicConfig(
    level=os.getenv("LOG_LEVEL", "INFO"),
    format="%(asctime)s %(levelname)s [worker] %(message)s",
)
logger = logging.getLogger(__name__)

REDIS_URL = os.getenv("REDIS_URL", "redis://redis:6379/0")
START_QUEUE = os.getenv("START_QUEUE", "avatar:sessions:start")
STOP_QUEUE = os.getenv("STOP_QUEUE", "avatar:sessions:stop")

MEDIAMTX_HOST = os.getenv("MEDIAMTX_HOST", "mediamtx")
MEDIAMTX_RTSP_PORT = int(os.getenv("MEDIAMTX_RTSP_PORT", "8554"))
WORKER_RTSP_USER = os.getenv("WORKER_RTSP_USER", "worker")
WORKER_RTSP_PASS = os.getenv("WORKER_RTSP_PASS", "worker-pass")
WORKER_MODE = os.getenv("WORKER_MODE", "passthrough")

RUNNING = True


@dataclass
class SessionProcess:
    session_id: str
    process: subprocess.Popen
    mode: str


sessions: dict[str, SessionProcess] = {}


def _rtsp_url(session_id: str, direction: str) -> str:
    return (
        f"rtsp://{WORKER_RTSP_USER}:{WORKER_RTSP_PASS}@{MEDIAMTX_HOST}:{MEDIAMTX_RTSP_PORT}"
        f"/avatar/{session_id}/{direction}"
    )


def _build_passthrough_cmd(session_id: str) -> list[str]:
    input_url = _rtsp_url(session_id, "in")
    output_url = _rtsp_url(session_id, "out")
    return [
        "ffmpeg",
        "-hide_banner",
        "-loglevel",
        "warning",
        "-rtsp_transport",
        "tcp",
        "-i",
        input_url,
        "-map",
        "0:v:0",
        "-map",
        "0:a:0?",
        "-c",
        "copy",
        "-f",
        "rtsp",
        "-rtsp_transport",
        "tcp",
        output_url,
    ]


def _build_soulx_cmd(session_id: str) -> list[str]:
    input_url = _rtsp_url(session_id, "in")
    output_url = _rtsp_url(session_id, "out")
    return [
        "python",
        "-m",
        "app.soulx_runner",
        "--session-id",
        session_id,
        "--input-rtsp",
        input_url,
        "--output-rtsp",
        output_url,
    ]


def _start_session(session_id: str, mode: str) -> None:
    if session_id in sessions:
        logger.info("session %s already running", session_id)
        return

    selected_mode = mode if mode in {"passthrough", "soulx"} else WORKER_MODE
    cmd = _build_passthrough_cmd(session_id) if selected_mode == "passthrough" else _build_soulx_cmd(session_id)

    logger.info("starting session=%s mode=%s", session_id, selected_mode)
    proc = subprocess.Popen(cmd)
    sessions[session_id] = SessionProcess(session_id=session_id, process=proc, mode=selected_mode)


def _stop_session(session_id: str) -> None:
    item = sessions.pop(session_id, None)
    if item is None:
        logger.info("session %s not running", session_id)
        return

    logger.info("stopping session=%s", session_id)
    item.process.terminate()
    try:
        item.process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        item.process.kill()


def _poll_exits() -> None:
    dead_sessions: list[str] = []
    for sid, item in sessions.items():
        rc = item.process.poll()
        if rc is not None:
            logger.warning("session=%s exited with code=%s", sid, rc)
            dead_sessions.append(sid)

    for sid in dead_sessions:
        sessions.pop(sid, None)


def _signal_handler(signum, _frame) -> None:
    global RUNNING
    logger.info("received signal=%s, shutting down", signum)
    RUNNING = False


def _parse_message(raw: bytes) -> dict | None:
    try:
        return json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        logger.exception("invalid queue payload")
        return None


def main() -> None:
    redis_client = Redis.from_url(REDIS_URL)

    signal.signal(signal.SIGINT, _signal_handler)
    signal.signal(signal.SIGTERM, _signal_handler)

    logger.info("worker started with mode=%s", WORKER_MODE)

    while RUNNING:
        _poll_exits()

        stop_msg = redis_client.brpop(STOP_QUEUE, timeout=1)
        if stop_msg:
            payload = _parse_message(stop_msg[1])
            if payload and payload.get("action") == "stop":
                _stop_session(payload.get("session_id", ""))
            continue

        start_msg = redis_client.brpop(START_QUEUE, timeout=1)
        if start_msg:
            payload = _parse_message(start_msg[1])
            if payload and payload.get("action") == "start":
                _start_session(
                    session_id=payload.get("session_id", ""),
                    mode=payload.get("worker_mode", WORKER_MODE),
                )

        time.sleep(0.05)

    for sid in list(sessions):
        _stop_session(sid)

    redis_client.close()


if __name__ == "__main__":
    main()
