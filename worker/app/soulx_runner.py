import argparse
import logging
import time


logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s [soulx-runner] %(message)s")
logger = logging.getLogger(__name__)


def main() -> None:
    parser = argparse.ArgumentParser(description="SoulX worker adapter entrypoint")
    parser.add_argument("--session-id", required=True)
    parser.add_argument("--input-rtsp", required=True)
    parser.add_argument("--output-rtsp", required=True)
    args = parser.parse_args()

    logger.info("session=%s input=%s output=%s", args.session_id, args.input_rtsp, args.output_rtsp)
    logger.info("replace this module with SoulX-FlashHead streaming inference pipeline")

    # Placeholder process loop so supervisor can keep the process alive.
    while True:
        time.sleep(5)


if __name__ == "__main__":
    main()
