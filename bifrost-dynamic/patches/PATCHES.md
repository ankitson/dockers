# Bifrost Dynamic Patches

Git patches in this directory apply after the upstream Bifrost checkout and
before compiling the local dynamic-plugin image.

- `list-models-partial-timeout.patch` lets model listing return partial provider
  results when a provider stalls instead of failing the whole request.

Keep these files exportable from a normal Git or `jj` authored stack.
