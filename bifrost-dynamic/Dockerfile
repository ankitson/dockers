ARG BIFROST_REF=transports/v1.6.4
ARG VERSION=1.6.4

FROM node:25-alpine3.23 AS ui-builder
ARG BIFROST_REF
WORKDIR /src
RUN apk add --no-cache git ca-certificates
RUN git clone https://github.com/maximhq/bifrost.git . && git checkout "$BIFROST_REF"
WORKDIR /src/ui
RUN npm ci
RUN npm run build-enterprise

FROM golang:1.26.4-alpine3.23 AS builder
ARG BIFROST_REF
ARG VERSION
WORKDIR /src
RUN apk add --no-cache git ca-certificates gcc musl-dev sqlite-dev binutils binutils-gold
RUN git clone https://github.com/maximhq/bifrost.git . && git checkout "$BIFROST_REF"
COPY patches/ /patches/
RUN git apply \
  /patches/list-models-partial-timeout.patch \
  /patches/pass-chat-template-kwargs.patch \
  /patches/add-reasoning-content-field.patch \
  /patches/preserve-custom-provider-hosted-tools.patch \
  /patches/allow-custom-provider-vk-wildcard.patch
COPY --from=ui-builder /src/ui/out ./transports/bifrost-http/ui

ENV CGO_ENABLED=1 GOOS=linux GOWORK=off
WORKDIR /src/transports
RUN go mod edit \
  -replace github.com/maximhq/bifrost/core=/src/core \
  -replace github.com/maximhq/bifrost/framework=/src/framework \
  -replace github.com/maximhq/bifrost/plugins/compat=/src/plugins/compat \
  -replace github.com/maximhq/bifrost/plugins/governance=/src/plugins/governance && \
  go mod tidy && \
  go mod download
RUN go build \
  -ldflags="-w -s -X main.Version=v${VERSION}" \
  -a -trimpath \
  -o /out/main \
  ./bifrost-http

# Build plugins within the transport module so they share the exact same
# go.mod / go.sum resolution as the main binary.  This prevents the
# "plugin was built with a different version of package" error at load time.
COPY policy-model-suffix /src/transports/plugins/policy-model-suffix/
COPY embedding-task-prefix /src/transports/plugins/embedding-task-prefix/
COPY normalizer /src/transports/plugins/normalizer/
RUN rm -f /src/transports/plugins/*/go.mod /src/transports/plugins/*/go.sum

RUN go build -buildmode=plugin -trimpath -ldflags="-w -s" \
  -o /out/plugins/model-policy-suffix.so \
  ./plugins/policy-model-suffix/

RUN go build -buildmode=plugin -trimpath -ldflags="-w -s" \
  -o /out/plugins/embedding-task-prefix.so \
  ./plugins/embedding-task-prefix/

RUN go build -buildmode=plugin -trimpath -ldflags="-w -s" \
  -o /out/plugins/normalizer.so \
  ./plugins/normalizer/

FROM alpine:3.23.4
WORKDIR /app
RUN apk add --no-cache musl libgcc ca-certificates zlib sqlite-libs && \
  adduser -D -s /bin/sh appuser

COPY --from=builder /out/main /app/main
COPY --from=builder /out/plugins /app/plugins
COPY --from=builder /src/transports/docker-entrypoint.sh /app/docker-entrypoint.sh

ARG ARG_APP_PORT=8080
ARG ARG_APP_HOST=0.0.0.0
ARG ARG_LOG_LEVEL=info
ARG ARG_LOG_STYLE=json
ARG ARG_APP_DIR=/app/data

ENV APP_PORT=$ARG_APP_PORT \
  APP_HOST=$ARG_APP_HOST \
  LOG_LEVEL=$ARG_LOG_LEVEL \
  LOG_STYLE=$ARG_LOG_STYLE \
  APP_DIR=$ARG_APP_DIR \
  GOGC="" \
  GOMEMLIMIT=""

RUN mkdir -p "$APP_DIR/logs" && \
  chown -R appuser:appuser /app && \
  chmod +x /app/docker-entrypoint.sh

USER appuser
VOLUME ["${APP_DIR}"]
EXPOSE $APP_PORT
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:${APP_PORT}/health || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/main"]
