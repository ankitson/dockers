set shell := ["bash", "-euo", "pipefail", "-c"]

IMAGE := "ankit/mcpproxy:0.35.0"
BIFROST_DYNAMIC_IMAGE := "ankit/bifrost-dynamic:local"

build:
  docker build --tag {{IMAGE}} ./mcpproxy

version: build
  docker run --rm --entrypoint mcpproxy {{IMAGE}} --version

bifrost-plugin-test:
  cd bifrost-dynamic/policy-model-suffix && go test ./...

bifrost-dynamic-build: bifrost-plugin-test
  docker build --tag {{BIFROST_DYNAMIC_IMAGE}} ./bifrost-dynamic
