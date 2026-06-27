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
  `order=...`, `allow_fallbacks=...`, dotted keys such as
  `reasoning.effort=high`, query-style suffixes such as
  `[?provider.only=digitalocean&reasoning.effort=high]`, and exact JSON object
  suffixes such as
  `[{"provider":{"only":["digitalocean"],"allow_fallbacks":false},"reasoning":{"effort":"high"}}]`.
  `provider=...` / `provider.only=...` imply `allow_fallbacks:false` unless
  explicitly overridden. Raw JSON is not mutated, so include
  `"allow_fallbacks":false` there when the intent is a hard provider pin.
- Verification: plugin unit tests pass, the Docker image builds as
  `ankit/bifrost-dynamic:local`, and the running devserver Bifrost reports the
  plugin active. End-to-end JSON suffix checks proved impossible providers fail
  at OpenRouter routing, and a DigitalOcean/ZDR JSON suffix produced OpenRouter
  generation `gen-1782519262-Agsv4Zb3PRmoKoh0DiHy` with
  `provider_name: DigitalOcean` and `preset_id:null`.

## 2026-06-02 - MCPProxy personal-edition image
- Goal: package MCPProxy as a reproducible local image for the devserver MCP gateway.
- Decision: use the official personal-edition `v0.35.0` Linux AMD64 artifact rather than the upstream server-edition Dockerfile.
- Decision: seed `/data/mcp_config.json` only when the persistent volume is empty; live state remains MCPProxy-managed after first boot.
- Decision: include Bash and Node.js/npm in the runtime image because MCPProxy's stdio launcher uses `/bin/bash` and client configs include stdio MCP servers launched with `npx`.
- Next step: build the image and deploy it from the devserver gateway branch.
