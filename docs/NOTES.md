# Dockers Notes

## 2026-06-02 - MCPProxy personal-edition image
- Goal: package MCPProxy as a reproducible local image for the devserver MCP gateway.
- Decision: use the official personal-edition `v0.35.0` Linux AMD64 artifact rather than the upstream server-edition Dockerfile.
- Decision: seed `/data/mcp_config.json` only when the persistent volume is empty; live state remains MCPProxy-managed after first boot.
- Next step: build the image and deploy it from the devserver gateway branch.
