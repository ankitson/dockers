# Dockers Notes

## 2026-07-01

### OpenClaw image patch for Bifrost embedding passthrough
#### Goal
- Make the npm-installed OpenClaw image compatible with the local Bifrost embedding shim without forking the whole OpenClaw package build.
#### Discovery
- The running `ankit/openclaw:local` image installs OpenClaw from npm and executes the bundled `dist/registry-*.js` runtime, so editing the checkout under `projects/external-repo/openclaw` would not affect production.
- OpenClaw already supports custom memory-search headers, but its OpenAI-compatible embedding adapter serializes `input_type` only as a top-level field, which Bifrost's OpenAI embeddings route drops before plugins run.
#### Decision
- Added a post-install image patch that rewrites the bundled OpenClaw embedding adapter so `input_type` is sent as `extra_params.input_type` whenever `x-bf-passthrough-extra-params: true` is configured.
#### Verification
- The image patch script fails closed if the expected upstream snippet changes, forcing an intentional review on future OpenClaw upgrades.

### Bifrost reusable embedding task-prefix shim
#### Goal
- Add a reusable Bifrost plugin that can normalize embedding input text for models that require task prefixes rather than OpenAI-style metadata fields.
#### Discovery
- Bifrost exposes embedding text plus provider/model metadata and `EmbeddingParameters.ExtraParams` to plugins before provider serialization.
- Ollama's OpenAI-compatible embeddings endpoint for `nomic-embed-text` ignores `input_type`, but changing the literal text prefix changes the resulting vector.
#### Decision
- Added `bifrost-dynamic/embedding-task-prefix`, a config-driven plugin that matches embedding requests by provider/model and rewrites each input string using a role-to-prefix map keyed by `input_type`.
- Defaulted the plugin to no-op unless a rule matches and an `input_type` value has a configured prefix, so it is safe to leave enabled globally.
#### Verification
- Unit tests cover config normalization, query/document rewriting, batched inputs, already-prefixed inputs, and unmatched requests.

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
