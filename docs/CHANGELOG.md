# Dockers Changelog

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
