# Dockers Changelog

## 2026-06-26

### Bifrost dynamic model-policy image
- Added `bifrost-dynamic/`, a custom Bifrost image that builds the upstream
  HTTP gateway dynamically and includes `/app/plugins/model-policy-suffix.so`.
- Added `model-policy-suffix`, a Bifrost plugin that maps trailing model suffix
  directives like `[zdr,provider=digitalocean]` into OpenRouter provider-routing
  request fields.
- Added `bifrost-plugin-test` and `bifrost-dynamic-build` Just recipes.

## 2026-06-02

### MCPProxy stdio runtime
- Added Bash, Node.js, and npm to the MCPProxy runtime image so stdio MCP
  servers launched via `npx` can run inside the gateway container.

### MCPProxy image
- Added a pinned Alpine-based MCPProxy personal-edition image.
- Verified the upstream `v0.35.0` tarball with its SHA-256 checksum during the build.
- Added a seed-once entrypoint so live MCPProxy configuration and OAuth state persist in `/data`.
