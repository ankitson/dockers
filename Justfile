set shell := ["bash", "-euo", "pipefail", "-c"]

IMAGE := "ankit/mcpproxy:0.35.0"

build:
  docker build --tag {{IMAGE}} ./mcpproxy

version: build
  docker run --rm --entrypoint mcpproxy {{IMAGE}} --version
