set shell := ["bash", "-euo", "pipefail", "-c"]

IMAGE := "ankit/mcpproxy:latest"
BIFROST_DYNAMIC_IMAGE := "ankit/bifrost-dynamic:latest"

build:
  docker build --tag {{IMAGE}} ./mcpproxy

version: build
  docker run --rm --entrypoint mcpproxy {{IMAGE}} --version

bifrost-plugin-test:
  cd bifrost-dynamic/policy-model-suffix && go test ./...
  cd bifrost-dynamic/embedding-task-prefix && go test ./...

bifrost-dynamic-build: bifrost-plugin-test
  docker build --tag {{BIFROST_DYNAMIC_IMAGE}} ./bifrost-dynamic

patches-list:
  uv run python bin/patch-stack.py list

patches-check:
  uv run python bin/patch-stack.py check
