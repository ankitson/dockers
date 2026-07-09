# OpenClaw Patches

These patches are applied inside the `ankit/openclaw` image build after the
OpenClaw npm package is installed.

- `patch-openclaw-bifrost-embeddings.mjs` rewrites the bundled embedding adapter
  only when the exact expected upstream snippet is present.
- `patch-export-session-assets.sh` creates the missing packaged asset path and
  verifies the expected template exists.

Both are tracked in the root `patches.toml` manifest.
