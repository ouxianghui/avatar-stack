import base64
import json
import logging
import os
import signal
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from urllib.parse import quote

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

MEDIAMTX_CONTROL_API_BASE = os.getenv("MEDIAMTX_CONTROL_API_BASE", "http://mediamtx:9997").rstrip("/")
MEDIAMTX_API_USER = os.getenv("MEDIAMTX_API_USER", "")
MEDIAMTX_API_PASS = os.getenv("MEDIAMTX_API_PASS", "")
# <= 0: wait until publisher or STOP (no time limit).
WAIT_PUBLISHER_TIMEOUT_S = float(os.getenv("WAIT_PUBLISHER_TIMEOUT_S", "300"))
WAIT_PUBLISHER_NO_PUBLISHER_RETRY_S = float(os.getenv("WAIT_PUBLISHER_NO_PUBLISHER_RETRY_S", "15"))
WORKER_RETRY_DELAY_S = float(os.getenv("WORKER_RETRY_DELAY_S", "8"))

RUNNING = True


@dataclass
class SessionProcess:
    session_id: str
    process: subprocess.Popen
    mode: str


@dataclass
class PublisherWait:
    selected_mode: str
    deadline: float


sessions: dict[str, SessionProcess] = {}
stopped_sessions: set[str] = set()
pending_restarts: dict[str, tuple[str, float]] = {}
publisher_waits: dict[str, PublisherWait] = {}


def _input_path_name(session_id: str) -> str:
    return f"avatar/{session_id}/in"


def _mediamtx_http_json(url: str) -> dict | None:
    req = urllib.request.Request(url)
    if MEDIAMTX_API_USER or MEDIAMTX_API_PASS:
        token = base64.b64encode(f"{MEDIAMTX_API_USER}:{MEDIAMTX_API_PASS}".encode()).decode()
        req.add_header("Authorization", f"Basic {token}")
    try:
        with urllib.request.urlopen(req, timeout=5) as resp:
            return json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        body = e.read().decode(errors="replace")
        logger.warning("mediamtx HTTP %s %s: %s", e.code, url.split("?", 1)[0], body[:300])
        return None
    except urllib.error.URLError:
        logger.exception("mediamtx request failed url=%s", url)
        return None


def _mediamtx_fetch_input_path_row(session_id: str) -> dict | None:
    name = _input_path_name(session_id)
    q = urllib.parse.urlencode({"page": "0", "itemsPerPage": "500"})
    url = f"{MEDIAMTX_CONTROL_API_BASE}/v3/paths/list?{q}"
    data = _mediamtx_http_json(url)
    if not data or data.get("status") == "error":
        return None
    for it in data.get("items") or []:
        if isinstance(it, dict) and it.get("name") == name:
            return it
    return None


def _path_has_publisher(path_row: dict | None) -> bool:
    if not path_row or path_row.get("status") == "error":
        return False
    if path_row.get("available") is not True:
        return False
    src = path_row.get("source")
    if not isinstance(src, dict):
        return False
    return bool(src.get("type"))


def _rtsp_url(session_id: str, direction: str) -> str:
    u = quote(WORKER_RTSP_USER, safe="")
    p = quote(WORKER_RTSP_PASS, safe="")
    return (
        f"rtsp://{u}:{p}@{MEDIAMTX_HOST}:{MEDIAMTX_RTSP_PORT}"
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
        "0:v:0?",
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


def _parse_message(raw: bytes) -> dict | None:
    try:
        return json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        logger.exception("invalid queue payload")
        return None


def _stop_session(session_id: str) -> None:
    stopped_sessions.add(session_id)
    pending_restarts.pop(session_id, None)
    publisher_waits.pop(session_id, None)

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


def _register_session_start(session_id: str, mode: str) -> None:
    if not str(session_id).strip():
        logger.warning("start session ignored: empty session_id")
        return

    if session_id in sessions:
        logger.info("session %s already running", session_id)
        return

    if session_id in publisher_waits:
        logger.info("session %s already waiting for publisher on /in", session_id)
        return

    stopped_sessions.discard(session_id)
    pending_restarts.pop(session_id, None)

    selected_mode = mode if mode in {"passthrough", "soulx"} else WORKER_MODE
    if selected_mode not in {"passthrough", "soulx"}:
        logger.warning("start session ignored: unsupported mode=%s", mode)
        return

    if WAIT_PUBLISHER_TIMEOUT_S > 0:
        deadline = time.monotonic() + WAIT_PUBLISHER_TIMEOUT_S
        logger.info(
            "session=%s waiting for publisher on /in (timeout %.0fs)",
            session_id,
            WAIT_PUBLISHER_TIMEOUT_S,
        )
    else:
        deadline = float("inf")
        logger.info("session=%s waiting for publisher on /in (no timeout)", session_id)

    publisher_waits[session_id] = PublisherWait(selected_mode=selected_mode, deadline=deadline)


def _spawn_session_process(session_id: str, selected_mode: str) -> None:
    cmd = _build_passthrough_cmd(session_id) if selected_mode == "passthrough" else _build_soulx_cmd(session_id)

    logger.info("starting session=%s mode=%s", session_id, selected_mode)
    proc = subprocess.Popen(cmd)
    sessions[session_id] = SessionProcess(session_id=session_id, process=proc, mode=selected_mode)


def _tick_publisher_waits() -> None:
    now = time.monotonic()
    for session_id, wait in list(publisher_waits.items()):
        if session_id in stopped_sessions or session_id in sessions:
            publisher_waits.pop(session_id, None)
            continue

        if wait.deadline != float("inf") and now >= wait.deadline:
            publisher_waits.pop(session_id, None)
            if session_id not in stopped_sessions:
                pending_restarts[session_id] = (
                    wait.selected_mode,
                    now + WAIT_PUBLISHER_NO_PUBLISHER_RETRY_S,
                )
                logger.warning(
                    "session=%s no publisher on /in within %.0fs; will retry in %.0fs",
                    session_id,
                    WAIT_PUBLISHER_TIMEOUT_S,
                    WAIT_PUBLISHER_NO_PUBLISHER_RETRY_S,
                )
            continue

        row = _mediamtx_fetch_input_path_row(session_id)
        if not _path_has_publisher(row):
            continue

        publisher_waits.pop(session_id, None)
        logger.info("session=%s publisher present on /in", session_id)
        _spawn_session_process(session_id, wait.selected_mode)


def _poll_exits() -> None:
    now = time.monotonic()
    for sid, item in list(sessions.items()):
        rc = item.process.poll()
        if rc is None:
            continue
        sessions.pop(sid, None)
        logger.warning("session=%s exited with code=%s", sid, rc)
        if sid in stopped_sessions:
            continue
        if rc != 0 and item.mode in {"passthrough", "soulx"}:
            pending_restarts[sid] = (item.mode, now + WORKER_RETRY_DELAY_S)
            logger.info("session=%s will retry in %.0fs", sid, WORKER_RETRY_DELAY_S)


def _apply_pending_restarts() -> None:
    now = time.monotonic()
    for sid, (mode, when) in list(pending_restarts.items()):
        if when > now:
            continue
        pending_restarts.pop(sid, None)
        if sid in stopped_sessions or sid in sessions or sid in publisher_waits:
            continue
        _register_session_start(sid, mode)


def _signal_handler(signum, _frame) -> None:
    global RUNNING
    logger.info("received signal=%s, shutting down", signum)
    RUNNING = False


def main() -> None:
    redis_client = Redis.from_url(REDIS_URL)

    signal.signal(signal.SIGINT, _signal_handler)
    signal.signal(signal.SIGTERM, _signal_handler)

    logger.info("worker started with mode=%s", WORKER_MODE)

    while RUNNING:
        _poll_exits()
        _apply_pending_restarts()
        _tick_publisher_waits()

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
                _register_session_start(
                    str(payload.get("session_id", "")).strip(),
                    str(payload.get("worker_mode", WORKER_MODE)),
                )

        time.sleep(0.05)

    for sid in list(sessions):
        _stop_session(sid)

    redis_client.close()


if __name__ == "__main__":
    main()
