# Bifrost Dynamic Patches

Git patches in this directory apply after the upstream Bifrost checkout and
before compiling the local dynamic-plugin image.

- `list-models-partial-timeout.patch` lets model listing return partial provider
  results when a provider stalls instead of failing the whole request.
- `pass-chat-template-kwargs.patch` preserves chat-template arguments for
  passthrough and custom providers.
- `add-reasoning-content-field.patch` mirrors provider reasoning into the
  OpenAI-compatible `reasoning_content` field and preserves it through the
  upstream custom unmarshal and streaming-copy paths.
- `allow-custom-provider-vk-wildcard.patch` keeps `allowed_models: ["*"]`
  truly unrestricted for explicitly granted custom providers, including valid
  aliases such as LM Studio's `current` that are absent from `/v1/models`.
- `preserve-custom-provider-hosted-tools.patch` corrects Bifrost's custom-provider
  context flag for providers based on a standard wire protocol, then prevents
  the compatibility plugin's public-model allowlist from deleting Responses
  hosted tools such as `web_search` before that custom provider can handle them.
  Its regression test also locks in filtering for standard providers. The image
  replaces the upstream `core`, `framework`, `plugins/compat`, and
  `plugins/governance` Go modules with the patched checkout so every patch is
  present in the compiled transport.

Keep these files exportable from a normal Git or `jj` authored stack.
