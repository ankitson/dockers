# syntax=docker/dockerfile:1.7

FROM alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d AS downloader

ARG MCPPROXY_VERSION=latest
ARG MCPPROXY_SHA256=

RUN apk add --no-cache ca-certificates curl tar \
  && set -eu; \
    if [ "${MCPPROXY_VERSION}" = "latest" ]; then \
      resolved_version="latest"; \
      download_url="https://github.com/smart-mcp-proxy/mcpproxy-go/releases/latest/download/mcpproxy-latest-linux-amd64.tar.gz"; \
    else \
      resolved_version="${MCPPROXY_VERSION}"; \
      download_url="https://github.com/smart-mcp-proxy/mcpproxy-go/releases/download/v${resolved_version}/mcpproxy-${resolved_version}-linux-amd64.tar.gz"; \
    fi; \
    echo "Downloading MCPProxy ${resolved_version} from ${download_url}"; \
    curl --fail --location --silent --show-error "${download_url}" --output /tmp/mcpproxy.tar.gz; \
    if [ -n "${MCPPROXY_SHA256}" ]; then \
      echo "${MCPPROXY_SHA256}  /tmp/mcpproxy.tar.gz" | sha256sum -c -; \
    fi \
  && tar -xzf /tmp/mcpproxy.tar.gz -C /tmp \
  && install -m 0755 /tmp/mcpproxy /usr/local/bin/mcpproxy

FROM alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d

RUN apk add --no-cache bash ca-certificates nodejs npm \
  && addgroup -g 1000 mcpproxy \
  && adduser -D -u 1000 -G mcpproxy -h /data mcpproxy \
  && mkdir -p /data \
  && chown mcpproxy:mcpproxy /data

COPY --from=downloader /usr/local/bin/mcpproxy /usr/local/bin/mcpproxy
COPY --chmod=755 entrypoint.sh /usr/local/bin/mcpproxy-entrypoint

ENV HOME=/data \
    HEADLESS=1 \
    MCPPROXY_TELEMETRY=false

USER mcpproxy
WORKDIR /data

EXPOSE 3130

ENTRYPOINT ["mcpproxy-entrypoint"]
CMD ["serve"]
