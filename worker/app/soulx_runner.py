"""
SoulX-FlashHead RTSP adapter (https://github.com/Soul-AILab/SoulX-FlashHead).

Upstream ``flash_head/inference.py`` opens ``flash_head/configs/infer_params.yaml``
relative to the process cwd; this module changes into the repo root before importing.

Set ``SOULX_DRY_RUN=1`` (default) to keep a idle loop without loading PyTorch (slim / CPU images).
For real inference use ``Dockerfile.soulx``, mount model weights, and set ``SOULX_DRY_RUN=0``.
"""

from __future__ import annotations

import argparse
import logging
import os
import queue
import shutil
import subprocess
import sys
import threading
import time
from collections import deque
from pathlib import Path

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s [soulx-runner] %(message)s")
logger = logging.getLogger(__name__)


def _truthy(name: str, default: str = "0") -> bool:
    return os.getenv(name, default).strip().lower() in {"1", "true", "yes", "on"}


def _repo_root() -> Path:
    env = os.getenv("SOULX_REPO_ROOT", "").strip()
    if env:
        return Path(env).resolve()
    return (Path(__file__).resolve().parent.parent / "third_party" / "SoulX-FlashHead").resolve()


def _run_ffmpeg_cond_frame(input_rtsp: str, out_png: Path) -> None:
    out_png.parent.mkdir(parents=True, exist_ok=True)
    cmd = [
        "ffmpeg",
        "-hide_banner",
        "-loglevel",
        "error",
        "-nostdin",
        "-y",
        "-rtsp_transport",
        "tcp",
        "-i",
        input_rtsp,
        "-frames:v",
        "1",
        str(out_png),
    ]
    subprocess.run(cmd, check=True)


def _pcm_reader_thread(proc: subprocess.Popen, out_q: queue.Queue[bytes], stop: threading.Event) -> None:
    assert proc.stdout is not None
    try:
        while not stop.is_set():
            chunk = proc.stdout.read(4096)
            if not chunk:
                break
            out_q.put(chunk)
    finally:
        try:
            proc.stdout.close()
        except OSError:
            pass


def _run_rtsp_flashhead(
    session_id: str,
    input_rtsp: str,
    output_rtsp: str,
    repo_root: Path,
) -> None:
    compile_on = os.getenv("SOULX_TORCH_COMPILE", "0").strip().lower() in {"1", "true", "yes", "on"}
    if not compile_on:
        os.environ["TORCHDYNAMO_DISABLE"] = "1"

    import numpy as np
    import torch

    if not torch.cuda.is_available():
        raise RuntimeError("CUDA is required for SoulX-FlashHead inference (torch.cuda.is_available() is False)")

    ckpt_dir = os.getenv("SOULX_CKPT_DIR", "/models/SoulX-FlashHead-1_3B").strip()
    wav2vec_dir = os.getenv("SOULX_WAV2VEC_DIR", "/models/wav2vec2-base-960h").strip()
    model_type = os.getenv("SOULX_MODEL_TYPE", "lite").strip().lower()
    if model_type not in {"lite", "pro", "pretrained"}:
        model_type = "lite"
    use_face_crop = _truthy("SOULX_USE_FACE_CROP", "0")
    seed_s = os.getenv("SOULX_SEED", "9999").strip()
    base_seed = int(seed_s) if seed_s.lstrip("-").isdigit() else 9999

    cond_image = os.getenv("SOULX_COND_IMAGE", "").strip()
    work = Path(os.getenv("SOULX_WORK_DIR", "/tmp/soulx-worker")).resolve() / session_id
    work.mkdir(parents=True, exist_ok=True)
    cond_path = Path(cond_image) if cond_image else work / "cond.png"

    prev_cwd = os.getcwd()
    sys.path.insert(0, str(repo_root))
    try:
        os.chdir(repo_root)
        import flash_head.src.pipeline.flash_head_pipeline as _fh_pipeline

        if not compile_on:
            _fh_pipeline.COMPILE_MODEL = False
            _fh_pipeline.COMPILE_VAE = False
            logger.info(
                "SoulX torch.compile disabled (SOULX_TORCH_COMPILE off, TORCHDYNAMO_DISABLE=1). "
                "Set SOULX_TORCH_COMPILE=1 only on images with CUDA link stubs."
            )

        from flash_head.inference import (
            get_audio_embedding,
            get_base_data,
            get_infer_params,
            get_pipeline,
            run_pipeline,
        )

        if not cond_image:
            logger.info("extracting condition frame from input RTSP to %s", cond_path)
            _run_ffmpeg_cond_frame(input_rtsp, cond_path)

        logger.info("loading pipeline model_type=%s ckpt=%s wav2vec=%s", model_type, ckpt_dir, wav2vec_dir)
        pipeline = get_pipeline(world_size=1, ckpt_dir=ckpt_dir, model_type=model_type, wav2vec_dir=wav2vec_dir)
        get_base_data(pipeline, cond_image_path_or_dir=str(cond_path), base_seed=base_seed, use_face_crop=use_face_crop)

        infer_params = get_infer_params()
        sample_rate = int(infer_params["sample_rate"])
        tgt_fps = int(infer_params["tgt_fps"])
        cached_audio_duration = float(infer_params["cached_audio_duration"])
        frame_num = int(infer_params["frame_num"])
        motion_frames_num = int(infer_params["motion_frames_num"])
        height = int(infer_params["height"])
        width = int(infer_params["width"])

        slice_len = frame_num - motion_frames_num
        human_speech_array_slice_len = slice_len * sample_rate // tgt_fps
        cached_audio_length_sum = int(sample_rate * cached_audio_duration)
        audio_end_idx = int(cached_audio_duration * tgt_fps)
        audio_start_idx = audio_end_idx - frame_num

        audio_cmd = [
            "ffmpeg",
            "-hide_banner",
            "-loglevel",
            "error",
            "-nostdin",
            "-rtsp_transport",
            "tcp",
            "-i",
            input_rtsp,
            "-vn",
            "-f",
            "s16le",
            "-ac",
            "1",
            "-ar",
            str(sample_rate),
            "-acodec",
            "pcm_s16le",
            "pipe:1",
        ]
        audio_proc = subprocess.Popen(audio_cmd, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
        pcm_queue: queue.Queue[bytes] = queue.Queue(maxsize=256)
        stop_reader = threading.Event()
        reader = threading.Thread(target=_pcm_reader_thread, args=(audio_proc, pcm_queue, stop_reader), daemon=True)
        reader.start()

        out_cmd = [
            "ffmpeg",
            "-hide_banner",
            "-loglevel",
            "warning",
            "-nostdin",
            "-f",
            "rawvideo",
            "-pix_fmt",
            "rgb24",
            "-s",
            f"{width}x{height}",
            "-r",
            str(tgt_fps),
            "-i",
            "pipe:0",
            "-rtsp_transport",
            "tcp",
            "-i",
            input_rtsp,
            "-map",
            "0:v:0",
            "-map",
            "1:a:0?",
            "-c:v",
            "libx264",
            "-preset",
            "veryfast",
            "-tune",
            "zerolatency",
            "-pix_fmt",
            "yuv420p",
            "-c:a",
            "aac",
            "-ar",
            str(sample_rate),
            "-shortest",
            "-f",
            "rtsp",
            output_rtsp,
        ]
        out_proc = subprocess.Popen(out_cmd, stdin=subprocess.PIPE, stderr=subprocess.PIPE)
        assert out_proc.stdin is not None

        audio_dq: deque[float] = deque([0.0] * cached_audio_length_sum, maxlen=cached_audio_length_sum)
        pcm_buf = bytearray()

        try:
            while True:
                if audio_proc.poll() is not None:
                    logger.warning("audio ffmpeg exited code=%s", audio_proc.returncode)
                    break
                if out_proc.poll() is not None:
                    err = (out_proc.stderr.read() or b"").decode(errors="replace") if out_proc.stderr else ""
                    logger.error("output ffmpeg exited code=%s stderr=%s", out_proc.returncode, err[:800])
                    break

                try:
                    pcm_buf.extend(pcm_queue.get_nowait())
                except queue.Empty:
                    time.sleep(0.01)
                    continue

                need = human_speech_array_slice_len * 2
                while len(pcm_buf) >= need:
                    block = pcm_buf[:need]
                    del pcm_buf[:need]
                    samples = np.frombuffer(bytes(block), dtype=np.int16).astype(np.float32) / 32768.0
                    audio_dq.extend(samples.tolist())
                    audio_array = np.array(audio_dq, dtype=np.float32)
                    audio_embedding = get_audio_embedding(pipeline, audio_array, audio_start_idx, audio_end_idx)
                    if audio_embedding is None:
                        logger.error("get_audio_embedding returned None")
                        continue
                    video = run_pipeline(pipeline, audio_embedding)
                    video = video[motion_frames_num:]
                    chunk = video.cpu().numpy().astype(np.uint8)
                    if chunk.size == 0:
                        continue
                    for i in range(chunk.shape[0]):
                        frame = chunk[i, :, :, :]
                        if frame.shape[-1] == 3:
                            out_proc.stdin.write(frame.tobytes())
                    out_proc.stdin.flush()
        finally:
            stop_reader.set()
            try:
                audio_proc.terminate()
                audio_proc.wait(timeout=3)
            except subprocess.TimeoutExpired:
                audio_proc.kill()
            try:
                out_proc.stdin.close()
            except OSError:
                pass
            try:
                out_proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                out_proc.kill()
    finally:
        os.chdir(prev_cwd)
        try:
            sys.path.remove(str(repo_root))
        except ValueError:
            pass
        try:
            shutil.rmtree(work, ignore_errors=True)
        except OSError:
            pass


def main() -> None:
    parser = argparse.ArgumentParser(description="SoulX-FlashHead RTSP worker adapter")
    parser.add_argument("--session-id", required=True)
    parser.add_argument("--input-rtsp", required=True)
    parser.add_argument("--output-rtsp", required=True)
    args = parser.parse_args()

    logger.info("session=%s input=%s output=%s", args.session_id, args.input_rtsp, args.output_rtsp)

    if _truthy("SOULX_DRY_RUN", "1"):
        logger.info(
            "SOULX_DRY_RUN is enabled (default). Disable with SOULX_DRY_RUN=0, use Dockerfile.soulx, "
            "clone submodule (worker/third_party/SoulX-FlashHead), and mount model weights."
        )
        while True:
            time.sleep(30)

    repo = _repo_root()
    if not (repo / "flash_head" / "inference.py").is_file():
        logger.error(
            "SoulX-FlashHead repo not found at %s. Run: git submodule update --init worker/third_party/SoulX-FlashHead",
            repo,
        )
        raise SystemExit(1)

    try:
        _run_rtsp_flashhead(args.session_id, args.input_rtsp, args.output_rtsp, repo)
    except Exception:
        logger.exception("soulx runner failed")
        raise


if __name__ == "__main__":
    main()
