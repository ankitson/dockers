# codex-oauth-proxy — turns a ChatGPT/Codex subscription into an OpenAI-compatible
# /v1/chat/completions endpoint that Bifrost registers as the custom `codex` provider.
#
# Runs our own proxy-server.mjs rather than the openai-oauth CLI's built-in HTTP
# server. Reason: openai-oauth's handleChatCompletionsRequest calls the Vercel AI
# SDK's generateText() fresh on every turn, fully re-translating message history
# with zero reasoning-continuity metadata — on the turn after a tool call, the
# model occasionally emits neither text nor a further tool call (confirmed via
# direct testing: reproducible ~100% of the time on a real multi-round agentic
# tool-calling loop, tests/test_reasoning_continuity.py). The obvious fix
# (store:true + previous_response_id, server-managed state) is NOT available —
# this ChatGPT/Codex backend rejects store:true outright ("Store must be set to
# false"). proxy-server.mjs instead implements the only mechanism this backend
# actually supports: stateless client-managed reasoning continuity — caching
# each tool-calling turn's raw reasoning+function_call items (via
# include:["reasoning.encrypted_content"]) and splicing them back into the
# input array on replay, keyed by that turn's stable tool_call ids. It reuses
# openai-oauth's own exported createCodexOAuthClient for OAuth/base-URL
# handling and nothing else.
#
# Wraps EvanZhouDev/openai-oauth (https://github.com/EvanZhouDev/openai-oauth)
# only for OAuth: it reads a Codex OAuth auth.json and auto-refreshes the
# access token against chatgpt.com/backend-api/codex. @openai/codex is
# installed only so createCodexOAuthClient can discover the local Codex client
# version offline (its --codex-version default).
#
# Auth is NOT baked into the image: mount a dedicated auth dir at /codex (see the
# devserver compose `codex-oauth` service). Use a SEPARATE `codex login` for this
# proxy so its rotating refresh token never contends with the host Codex CLI's.
FROM node:22-alpine

RUN npm install -g openai-oauth@latest @openai/codex@latest \
  && npm cache clean --force

COPY proxy-server.mjs /app/proxy-server.mjs

# Dedicated CODEX_HOME so model-version discovery and auth lookups stay self-contained.
ENV CODEX_HOME=/codex
WORKDIR /codex

EXPOSE 10531

# Bind to all interfaces so other mybridge services (Bifrost) can reach it.
ENV HOST=0.0.0.0
ENV PORT=10531
ENV OAUTH_FILE=/codex/auth.json
ENTRYPOINT ["node", "/app/proxy-server.mjs"]
