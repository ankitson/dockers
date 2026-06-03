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

if [ -n "${OPENCLAW_AGENT_SSH_KEY_REF:-}" ]; then
  install -d -m 700 "$HOME/.ssh"
  op read --force --file-mode 0600 --out-file "$HOME/.ssh/id_ed25519.tmp" \
    "$OPENCLAW_AGENT_SSH_KEY_REF" >/dev/null
  mv "$HOME/.ssh/id_ed25519.tmp" "$HOME/.ssh/id_ed25519"
  ssh-keygen -y -f "$HOME/.ssh/id_ed25519" > "$HOME/.ssh/id_ed25519.pub.tmp"
  mv "$HOME/.ssh/id_ed25519.pub.tmp" "$HOME/.ssh/id_ed25519.pub"
  chmod 600 "$HOME/.ssh/id_ed25519"
  chmod 644 "$HOME/.ssh/id_ed25519.pub"
fi

if [ -n "${OPENCLAW_BOOTSTRAP_PLUGIN_SPECS:-}" ]; then
  for spec in ${OPENCLAW_BOOTSTRAP_PLUGIN_SPECS}; do
    name="${spec%@*}"
    version="${spec##*@}"
    pkg="${OPENCLAW_STATE_DIR:-$HOME/.openclaw}/npm/node_modules/${name}/package.json"
    if [ ! -f "$pkg" ] || ! node -e 'const p=process.argv[1], v=process.argv[2]; process.exit(require(p).version === v ? 0 : 1)' "$pkg" "$version"; then
      openclaw plugins install "$spec" --pin >/tmp/openclaw-plugin-install.log 2>&1 || {
        cat /tmp/openclaw-plugin-install.log >&2
        exit 1
      }
    fi
  done
fi

if [ -f /run/openclaw/openclaw.config.patch.json ]; then
  openclaw config patch --file /run/openclaw/openclaw.config.patch.json
fi

exec openclaw gateway
