#!/bin/sh
set -eu

config_path="${MCPPROXY_CONFIG_PATH:-/data/mcp_config.json}"
seed_path="${MCPPROXY_SEED_PATH:-/seed/mcp_config.json}"

if [ ! -e "${config_path}" ]; then
  if [ ! -r "${seed_path}" ]; then
    echo "mcpproxy: missing initial config at ${seed_path}" >&2
    exit 1
  fi

  cp "${seed_path}" "${config_path}"
fi

exec mcpproxy \
  --config "${config_path}" \
  --data-dir /data \
  "$@"
