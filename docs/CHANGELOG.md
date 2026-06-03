# Dockers Changelog

## 2026-06-02

### MCPProxy stdio runtime
- Added Bash, Node.js, and npm to the MCPProxy runtime image so stdio MCP
  servers launched via `npx` can run inside the gateway container.

### MCPProxy image
- Added a pinned Alpine-based MCPProxy personal-edition image.
- Verified the upstream `v0.35.0` tarball with its SHA-256 checksum during the build.
- Added a seed-once entrypoint so live MCPProxy configuration and OAuth state persist in `/data`.
