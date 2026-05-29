# syntax=docker/dockerfile:1.7
#
# OpenClaw browser sidecar — chromium + Xvfb + x11vnc + noVNC.
# Vendored from upstream openclaw (scripts/docker/sandbox/Dockerfile.browser) so this
# setup is self-contained and does not depend on the external openclaw checkout.
# Exposes CDP (9222, auth-gated relay), VNC (5900) and noVNC web GUI (6080).

FROM debian:bookworm-slim@sha256:f9c6a2fd2ddbc23e336b6257a5245e31f996953ef06cd13a59fa0a1df2d5c252

LABEL org.openclaw.sandbox-browser.contract="2026-05-12-cdp-relay-auth"

ENV DEBIAN_FRONTEND=noninteractive

RUN --mount=type=cache,id=openclaw-sandbox-bookworm-apt-cache,target=/var/cache/apt,sharing=locked \
  --mount=type=cache,id=openclaw-sandbox-bookworm-apt-lists,target=/var/lib/apt,sharing=locked \
  apt-get update \
  && apt-get install -y --no-install-recommends \
    bash \
    ca-certificates \
    chromium \
    curl \
    fonts-liberation \
    fonts-noto-cjk \
    fonts-noto-color-emoji \
    git \
    jq \
    novnc \
    python3 \
    websockify \
    x11vnc \
    xvfb

COPY --chmod=755 entrypoint.sh /usr/local/bin/openclaw-sandbox-browser

RUN useradd --create-home --shell /bin/bash sandbox

# entrypoint uses HOME=/tmp/openclaw-home; pre-create it owned by sandbox so a fresh
# named volume mounted here inherits sandbox ownership (Docker seeds empty volumes
# from the image dir), letting the non-root sandbox user persist the chromium profile.
RUN mkdir -p /tmp/openclaw-home && chown sandbox:sandbox /tmp/openclaw-home

USER sandbox
WORKDIR /home/sandbox

EXPOSE 9222 5900 6080

CMD ["openclaw-sandbox-browser"]
