# syntax=docker/dockerfile:1.7

FROM alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d AS downloader

ARG MCPPROXY_VERSION=0.35.0
ARG MCPPROXY_SHA256=5fd8592469117fb4656a8b4d335bdf651219f210d4911a5849b6dcc510b2f97c

RUN apk add --no-cache ca-certificates curl tar \
  && curl --fail --location --silent --show-error \
    "https://github.com/smart-mcp-proxy/mcpproxy-go/releases/download/v${MCPPROXY_VERSION}/mcpproxy-${MCPPROXY_VERSION}-linux-amd64.tar.gz" \
    --output /tmp/mcpproxy.tar.gz \
  && echo "${MCPPROXY_SHA256}  /tmp/mcpproxy.tar.gz" | sha256sum -c - \
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
