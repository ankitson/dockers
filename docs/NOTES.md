# Dockers Notes

## 2026-07-14

### Bifrost VK wildcard semantics and 1.6.4 upgrade

#### Goal
- Make wildcard virtual-key grants reliable for LM Studio's dynamic `current`
  alias and eliminate manual SQLite policy edits.

#### Discovery
- Request `5cc21dc6-47c8-4a2f-b258-798938f9df90` was denied before provider
  dispatch because the dev VK's LM Studio policy contained
  `allowed_models=["*","current"]`.
- Bifrost's `WhiteList` contract requires `"*"` to be the only entry. The prior
  direct SQLite update bypassed `BeforeSave` validation, turning the list into a
  restricted list that matched neither the resolved model nor wildcard mode.
- Upstream issue #4318 is adjacent but not the direct cause: it repairs legacy
  rows stored as bare `*` instead of JSON. This row was valid JSON with invalid
  wildcard composition. The #4318 migration shipped in `transports/v1.6.4`.
- Even canonical `["*"]` was insufficient for callable custom-provider aliases
  absent from model discovery. The released model-catalog path only honored a
  custom-provider wildcard without catalog membership when `list_models` was
  disabled.
- Patching `/src/framework` had no runtime effect until the transport module
  replaced its released `framework` and `governance` dependencies with the
  patched checkout.

#### Decision
- Upgrade the image to `transports/v1.6.4` and retain model discovery.
- Treat a wildcard grant on an explicitly selected custom provider as true
  allow-all, matching `WhiteList`'s documented semantics. Standard providers
  keep catalog cross-checking.
- Rebase the existing `reasoning_content` patch over 1.6.4 and compile all
  patched local modules into the transport. This is a source build patch, not a
  new Bifrost plugin.

#### Verification
- All five source patches apply cleanly to `transports/v1.6.4`.
- Focused schema, stream-copy, model-catalog wildcard, and compatibility-plugin
  tests passed against the patched checkout.
- The image built successfully and the recreated Bifrost container is healthy.
- Both `lmstudio/current` and the provider-qualified loaded model completed
  through the dev VK while its persisted policy remained canonical `["*"]`.

### Bifrost compat parameter passthrough

#### Goal
- Stop Bifrost from silently deleting request parameters that are absent from a
  public model-catalog `supported_parameters` allowlist.

#### Discovery
- Upstream Bifrost exposes `PUT /api/config` for runtime client-config updates;
  that handler persists `client_config` to `config_client`, reloads the in-memory
  client config, and reloads the built-in compat plugin when compat flags change.
- For this deployment, the mounted `config/bifrost.config.json.tmpl` renders to
  `/app/data/config.json`. On container startup, Bifrost's `LoadConfig` loads the
  `client` section and calls `UpdateClientConfig` when the client-config hash
  differs from the row in `config_client`, so changing the template is enough for
  a recreated container to update the persisted `compat_should_drop_params` DB
  column.
- A template-only edit does not affect an already-running process until the
  service is recreated or `/api/config` is called.

#### Decision
- Set `client.compat.should_drop_params` to `false` in the devserver Bifrost
  config template and rendered live config.
- Leave the other compat features enabled: text-to-chat conversion,
  chat-to-Responses conversion, and parameter conversion.
- Treat this as a global Bifrost behavior change, not a codex-specific fix. The
  configured providers currently include `openrouter`, `anthropic`,
  `opencode-zen`, `openai`, `codex`, `nvidia`, `deepseek`, `nanogpt`,
  `unsloth`, `lmstudio`, `ollama`, `local-tts`, `speaches`, `audiocpp`,
  `nemotron-asr`, and `parakeet-asr`; any of them may now receive request fields
  Bifrost previously dropped.

#### Verification
- After rendering and recreating the service, verify with:
  `sqlite3 volumes/bifrost/config.db 'select compat_should_drop_params from config_client;'`.

## 2026-07-10

### Bifrost custom-provider hosted tools

#### Goal
- Preserve native Responses hosted tools when a custom OpenAI-compatible
  provider supports more than Bifrost's public model metadata declares.

#### Discovery
- Bifrost parsed `web_search` correctly and logged it on the incoming request,
  but the Codex OAuth proxy received zero tools.
- With `compat_should_drop_params` enabled, the compatibility plugin used the
  public `gpt-5.4-mini` catalog row. That row declares function tools but not
  hosted web search, so functions survived while `web_search` was silently
  deleted.
- Bifrost's `is-custom-provider` context flag was derived from the provider's
  base wire protocol. A custom provider based on OpenAI was therefore marked as
  non-custom, defeating the intended custom-provider compatibility branch.
- The transport module separately pins a released `plugins/compat` Go module.
  Patching the checkout alone had no runtime effect until the image build also
  replaced that module with `/src/plugins/compat`.
- A separate lossy path exists when a custom provider allows chat completions
  but disables Responses: Bifrost converts Responses to Chat and intentionally
  retains only function tools. Native Responses support must remain enabled for
  any provider expected to handle hosted tools.

#### Decision
- Derive the custom-provider flag by comparing the selected provider key with
  its resolved base wire provider, not by testing whether that base is standard.
  Skip catalog-based hosted-tool deletion only after Bifrost has selected that
  custom provider. Standard providers retain the existing filtering.
- Populate that flag before both non-streaming and streaming pre-LLM plugin
  pipelines; the provider worker runs too late to protect request parameters.
- Compile both the patched local `core` and `plugins/compat` modules into the
  transport binary.
- Keep both `responses` and `responses_stream` enabled on the Codex provider.

#### Verification
- The focused upstream test preserves function, `web_search`, and
  `web_search_preview` for custom providers and verifies that standard providers
  still remove the hosted tools.
- The patch applies cleanly to `transports/v1.6.3` and is built into the local
  dynamic image.

### Codex OAuth Responses behavior

#### Decision
- Keep the subscription backend explicitly stateless by forcing `store:false`.
- Forward reasoning effort even when encrypted reasoning continuity was not
  requested; treat encrypted content as a separate include capability.
- Preserve upstream usage-limit semantics as HTTP 429 so conformance clients can
  distinguish an account window from routing or provider failures.
- Relay streaming Responses directly from the upstream Codex SSE body. The
  non-stream path still aggregates output-item events because the final
  `response.completed` snapshot can omit output when `store:false`.
- Install `openai-oauth@latest` and `@openai/codex@latest`. At runtime, derive
  the Codex version from `codex --version` and send the matching first-party
  `User-Agent` and `originator`. The backend gates rollout discovery and Luna
  execution on that client identity even for an entitled OAuth account.
- Use `just codex-oauth-build-latest` (a no-cache build) when refreshing the
  image; Docker otherwise treats an old cached `@latest` install layer as valid.

#### Verification
- Hosted `web_search` arrives at the Codex adapter with one tool and produces
  native `web_search_call` events.
- Dynamic discovery returns `gpt-5.6-luna`, `gpt-5.6-terra`, and
  `gpt-5.6-sol`; all three completed low-effort smoke requests through Bifrost.
- The final gateway conformance run passes both qualified and bare progressive
  SSE checks as well as hosted search and MCP server-side execution.

## 2026-07-06

### Bifrost dynamic partial model-list patch
#### Goal
- Make the custom Bifrost image tolerate a slow provider during all-provider model discovery.
#### Decision
- Moved `bifrost-dynamic` from upstream `transports/v1.6.0` to `transports/v1.6.2`.
- Added `bifrost-dynamic/patches/list-models-partial-timeout.patch`.
- The patch makes `ListAllModels` collect provider fanout results for 10 seconds, return available models, and record provider statuses for success, failure, and timeout.
- The OpenAI-compatible list response keeps `object` and `data`, and adds optional top-level `bifrost` metadata for partial/provider status details.
- The Docker build now compiles the main transport and both local plugins against the same patched `/src/core` module.
#### Verification
- `just bifrost-dynamic-build` passes plugin tests and builds `ankit/bifrost-dynamic:latest`.
- Devserver Bifrost runs v1.6.2 with `model-policy-suffix` and `embedding-task-prefix` active.
- `/v1/models` and `/openai/v1/models` return in about 10 seconds and mark `unsloth` as timed out while preserving returned model data.

### Patch stack structure
#### Goal
- Make local monkey patches discoverable and maintainable without changing any running devserver containers.
#### Decision
- Use `jj` as an optional authoring workflow for long-lived upstream patch stacks, but keep Docker builds consuming plain files.
- Track every local patch in `patches.toml` with owner, upstream, apply point, reason, and removal condition.
- Keep source/package patch files under each image's `patches/` directory.
#### Verification
- `just patches-check` validates manifest structure and patch file presence without building images or touching containers.
#### Next steps
- Convert future Git-backed source patches to `git format-patch` output and apply them with `git am` where the upstream checkout supports it.

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
