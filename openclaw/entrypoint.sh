#!/usr/bin/env bash
# Single-purpose entrypoint: load secrets/config, then run the OpenClaw gateway.
# No sshd/supervisor needed — debug with `docker exec -it openclaw bash`.
set -euo pipefail

if [ -f /run/openclaw/openclaw.env ]; then
  set -a
  # shellcheck disable=SC1091
  . /run/openclaw/openclaw.env
  set +a
fi

exec openclaw gateway
