# Dockers Changelog

## 2026-07-14

### Bifrost 1.6.4 and custom-provider VK wildcard correctness

- Upgraded the dynamic image from `transports/v1.6.3` to `v1.6.4`, including
  upstream's repair migration for bare wildcard model-list rows from issue
  #4318.
- Added a focused source patch so `allowed_models: ["*"]` remains genuinely
  unrestricted for explicitly granted custom providers, including callable
  aliases such as LM Studio's `current` that are absent from `/v1/models`.
- Rebased the `reasoning_content` patch over 1.6.4's custom unmarshalling and
  citation fields, preserving the compatibility field during unmarshal and
  stream deep-copy operations with regression tests.
- Made the transport build replace patched local `core`, `framework`,
  `plugins/compat`, and `plugins/governance` modules so every source patch is
  present in the final binary.

### Bifrost dynamic deployment: disable compat parameter dropping

- Disabled Bifrost's global compatibility-plugin parameter dropping in the
  devserver Bifrost client config by setting
  `client.compat.should_drop_params: false`.
- This is a global behavior change for the deployed Bifrost instance: all
  providers can now receive unsupported or previously catalog-filtered request
  parameters instead of having Bifrost silently delete them before provider
  dispatch.

## 2026-07-10

### Bifrost dynamic: preserve hosted Responses tools for custom providers

- Added an upstream source patch that correctly marks protocol-based custom
  providers as custom, then prevents public-model compatibility metadata from
  deleting `web_search` and `web_search_preview` before they receive a request.
- Kept the existing fail-closed hosted-tool filtering for standard providers
  and added a focused upstream regression test for both branches.
- Registered the patch in the local patch-stack manifest and made the transport
  build replace both patched local `core` and `plugins/compat` modules.

### Codex OAuth proxy: Responses correctness

- Preserved `reasoning.effort` independently of whether encrypted reasoning was
  requested, while forwarding `reasoning.encrypted_content` only when included
  by the client.
- Mapped upstream subscription usage-limit failures to HTTP 429.
- Replaced buffered synthetic Responses streaming with direct upstream SSE relay,
  preserving real output-text deltas through Bifrost.
- Kept `openai-oauth` and Codex on `@latest`, derived the client version from the
  installed binary at runtime, and forwarded the first-party Codex client
  identity required for 5.6 rollout discovery/execution.
- Added a Just recipe for a repeatable proxy syntax check.

## 2026-07-06

### Bifrost dynamic: partial model-list fanout
- Updated `bifrost-dynamic` to upstream Bifrost `transports/v1.6.2`.
- Added a local source patch that bounds all-provider model-list collection at 10 seconds, returns available models, and reports provider statuses for failed or timed-out providers.
- Added OpenAI-compatible `bifrost.partial` and `bifrost.provider_statuses` metadata for `/openai/v1/models`.
- Fixed the Docker build so the transport binary and custom Go plugins compile against the same patched local core module.

### Patch stack structure
- Added `patches.toml` as the central manifest for local image/source patch stacks.
- Added `PATCHES.md` and per-image patch notes for the `jj` authoring plus plain-file Docker build workflow.
- Added `bin/patch-stack.py` with `just patches-list` and `just patches-check` structural validation commands.
- Moved the OpenClaw embedding patch under `openclaw/patches/` and made the export-session asset workaround an explicit patch script.

## 2026-07-01

### OpenClaw Bifrost embedding passthrough patch
- Added `openclaw/patch-openclaw-bifrost-embeddings.mjs`, a post-install runtime patch for the npm OpenClaw package.
- Updated the OpenClaw image build to apply that patch after `npm install -g openclaw@...`.
- The patch makes the OpenAI-compatible embedding adapter serialize `input_type` as `extra_params.input_type` when `x-bf-passthrough-extra-params: true` is configured, which matches Bifrost's supported extra-param transport.

### Bifrost embedding task-prefix plugin
- Added `bifrost-dynamic/embedding-task-prefix`, a Bifrost plugin that rewrites embedding text using configurable task prefixes based on provider/model matching and an `input_type` request field.
- Added unit tests for config parsing, single-string and batched embeddings, idempotent prefixing, and unmatched requests.
- Updated the custom Bifrost image and plugin test recipe to build and test `embedding-task-prefix.so` alongside `model-policy-suffix.so`.

## 2026-06-26

### Bifrost dynamic model-policy image
- Added `bifrost-dynamic/`, a custom Bifrost image that builds the upstream
  HTTP gateway dynamically and includes `/app/plugins/model-policy-suffix.so`.
- Added `model-policy-suffix`, a Bifrost plugin that maps trailing model suffix
  directives like `[zdr,provider=digitalocean]` into OpenRouter provider-routing
  request fields.
- Expanded `model-policy-suffix` to accept arbitrary OpenRouter request params
  from the model suffix via raw JSON object, quoted JSON, `json64:...`, or
  query/dotted-key forms while keeping the shorthand ZDR/provider syntax.
- Added `bifrost-plugin-test` and `bifrost-dynamic-build` Just recipes.

## 2026-06-02

### MCPProxy stdio runtime
- Added Bash, Node.js, and npm to the MCPProxy runtime image so stdio MCP
  servers launched via `npx` can run inside the gateway container.

### MCPProxy image
- Added a pinned Alpine-based MCPProxy personal-edition image.
- Verified the upstream `v0.35.0` tarball with its SHA-256 checksum during the build.
- Added a seed-once entrypoint so live MCPProxy configuration and OAuth state persist in `/data`.
