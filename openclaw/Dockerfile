# OpenClaw gateway image — thin layer over the devbox image so OpenClaw runs in
# the same toolchain/environment as the Hermes sandbox, without touching agent-devbox.
ARG DEVBOX_TAG=latest
FROM ankit/devbox:${DEVBOX_TAG}

# Pin Node to 22 LTS for OpenClaw only. Devbox ships Node 25, which crashes openclaw
# 2026.5.26 with `ERR_INVALID_STATE: A FileHandle object was closed during garbage
# collection` on session jsonl.lock cleanup (Node 25 promoted that deprecation to a
# hard error). Hermes/agent-devbox keep Node 25 — this pin is local to the openclaw image.
USER root
RUN apt-get purge -y nodejs npm \
 && rm -f /etc/apt/sources.list.d/nodesource.list \
 && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
 && apt-get install -y -q nodejs \
 && node --version | grep -q '^v22\.' \
 && npm install -g pnpm bun typescript

# Install OpenClaw globally as root so the binary lands in the image-wide
# node_modules (on ankit's PATH). Pinned for reproducibility — bump deliberately.
ARG OPENCLAW_VERSION=2026.5.28
ARG OPENCLAW_CODEX_VERSION=${OPENCLAW_VERSION}
RUN npm install -g openclaw@${OPENCLAW_VERSION} \
 && openclaw --version

COPY patch-openclaw-bifrost-embeddings.mjs /tmp/patch-openclaw-bifrost-embeddings.mjs
RUN node /tmp/patch-openclaw-bifrost-embeddings.mjs \
 && rm /tmp/patch-openclaw-bifrost-embeddings.mjs

# ACP runtime + harnesses for /acp spawn:
#   @openclaw/codex          — OpenClaw's Codex app-server harness plugin
#   @openclaw/acpx           — OpenClaw's ACP backend plugin
#   @earendil-works/pi-coding-agent — the pi coding agent CLI
#   pi-acp                   — ACP adapter for pi (spawns `pi --mode rpc` and bridges ACP JSON-RPC)
#   opencode-ai              — opencode coding agent CLI (acpx supports it natively as harness id "opencode")
# Mirrored in /projects/devdocker/node.sh so future devbox image rebuilds also include these.
RUN npm install -g \
      @openclaw/codex@${OPENCLAW_CODEX_VERSION} \
      @openclaw/acpx \
      @earendil-works/pi-coding-agent \
      pi-acp \
      opencode-ai \
 && pi --version 2>/dev/null \
 && pi-acp --version 2>/dev/null \
 && opencode --version 2>/dev/null \
 || true

# Docker Compose CLI plugin. Devbox (devbase.sh) ships `docker-ce-cli` but not the
# compose plugin; gilfoyle's ops loops run `docker compose pull/up` against the mounted
# homeserver/devserver stacks. Scoped to the openclaw image so other devbox consumers
# are untouched. The docker socket itself is bind-mounted by the compose service.
RUN apt-get update \
 && apt-get install -y -q docker-compose-plugin \
 && docker compose version \
 && rm -rf /var/lib/apt/lists/*

# Workaround for an OpenClaw packaging bug (≤2026.6.1): the /export-session slash handler
# opens dist/export-html/template.html, but the assets ship at
# dist/auto-reply/reply/export-html/. Without this the interaction throws ENOENT and hangs.
# Symlink the directory so the handler finds template.html/.css/.js/vendor. Remove once upstream fixes it.
RUN ln -sfn auto-reply/reply/export-html /usr/lib/node_modules/openclaw/dist/export-html \
 && test -e /usr/lib/node_modules/openclaw/dist/export-html/template.html

COPY entrypoint.sh /usr/local/bin/openclaw-entrypoint
RUN chmod +x /usr/local/bin/openclaw-entrypoint

USER ankit
WORKDIR /home/ankit

ENTRYPOINT ["/usr/local/bin/openclaw-entrypoint"]
