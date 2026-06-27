# Dockers Notes

## 2026-06-26 - Bifrost dynamic model-policy image
- Goal: avoid one OpenRouter preset per model/provider by making Bifrost own
  model-string policy.
- Added `bifrost-dynamic/`, a local Bifrost image built from upstream source at
  commit `e9d1f3a6bce0cacde76e361dac36a66f5034ca2e` without static linker flags,
  so Go `.so` plugins can load.
- Added the `model-policy-suffix` plugin. For OpenRouter models, it parses a
  trailing bracket suffix such as
  `deepseek/deepseek-v4-flash[zdr,provider=digitalocean]`, strips the suffix
  before provider routing, and injects OpenRouter `provider` routing fields via
  Bifrost `ExtraParams`.
- Supported suffix directives include bare `zdr`, `provider=...`, `only=...`,
  `order=...`, `allow_fallbacks=...`, and generic `key=value` passthrough into
  OpenRouter's `provider` object. `provider=...` implies
  `allow_fallbacks:false` unless explicitly overridden.
- Verification: plugin unit tests pass, the Docker image builds as
  `ankit/bifrost-dynamic:local`, and the running devserver Bifrost reports the
  plugin active.

## 2026-06-02 - MCPProxy personal-edition image
- Goal: package MCPProxy as a reproducible local image for the devserver MCP gateway.
- Decision: use the official personal-edition `v0.35.0` Linux AMD64 artifact rather than the upstream server-edition Dockerfile.
- Decision: seed `/data/mcp_config.json` only when the persistent volume is empty; live state remains MCPProxy-managed after first boot.
- Decision: include Bash and Node.js/npm in the runtime image because MCPProxy's stdio launcher uses `/bin/bash` and client configs include stdio MCP servers launched with `npx`.
- Next step: build the image and deploy it from the devserver gateway branch.
