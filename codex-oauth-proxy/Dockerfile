# codex-oauth-proxy — turns a ChatGPT/Codex subscription into an OpenAI-compatible
# /v1 endpoint that Bifrost registers as the custom `codex` provider.
#
# Wraps EvanZhouDev/openai-oauth (https://github.com/EvanZhouDev/openai-oauth):
# it reads a Codex OAuth auth.json, auto-refreshes the access token, and proxies
# to chatgpt.com/backend-api/codex/responses, exposing /v1/chat/completions,
# /v1/responses, and /v1/models. @openai/codex is installed only so the proxy can
# discover the local Codex client version offline (its --codex-version default).
#
# Auth is NOT baked into the image: mount a dedicated auth dir at /codex (see the
# devserver compose `codex-oauth` service). Use a SEPARATE `codex login` for this
# proxy so its rotating refresh token never contends with the host Codex CLI's.
FROM node:22-alpine

RUN npm install -g openai-oauth@latest @openai/codex@latest \
  && npm cache clean --force

# Dedicated CODEX_HOME so model-version discovery and auth lookups stay self-contained.
ENV CODEX_HOME=/codex
WORKDIR /codex

EXPOSE 10531

# Bind to all interfaces so other mybridge services (Bifrost) can reach it.
# --oauth-file pins the auth.json this proxy refreshes (its own, not the host's).
ENTRYPOINT ["openai-oauth"]
CMD ["--host", "0.0.0.0", "--port", "10531", "--oauth-file", "/codex/auth.json"]
