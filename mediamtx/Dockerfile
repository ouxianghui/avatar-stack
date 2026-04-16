# Standard bluenviron/mediamtx is scratch-based (no shell/curl). Hooks need a client binary + libc.
# Allow overriding source registry when Docker Hub is flaky:
#   MEDIAMTX_IMAGE=<reachable-registry>/bluenviron/mediamtx:1.17.1-ffmpeg
ARG MEDIAMTX_IMAGE=bluenviron/mediamtx:1.17.1-ffmpeg
FROM ${MEDIAMTX_IMAGE}
RUN apk add --no-cache curl
