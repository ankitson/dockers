# Patch Stacks

This repo keeps local compatibility patches close to the image that consumes
them. `jj` is the preferred authoring tool for long-lived stacks, but Docker
builds consume ordinary files: Git patches, package patch scripts, or tiny image
fixup scripts.

## Rules

- Code patches apply during image build, not container startup.
- Each patch is listed in `patches.toml` with an owner, reason, apply point, and
  removal condition.
- Git-backed upstreams should use `git format-patch` output when possible and
  apply with `git am` or `git apply`.
- Package-only upstreams may use fail-closed patch scripts when a clean Git
  patch cannot be applied before packaging.
- Runtime entrypoints may reconcile mutable state, but should not rewrite source
  code.

## Authoring With jj

Use `jj` in a scratch checkout of the upstream or in a colocated workspace when
you want to maintain a stack over time:

```sh
jj git clone https://github.com/maximhq/bifrost.git bifrost-patches
cd bifrost-patches
jj new main
# edit, then create one change per local delta
jj commit -m "bifrost: return partial model-list results on provider timeout"
```

When upstream moves:

```sh
jj git fetch
jj rebase -b <stack-bookmark> -d main
```

Export build artifacts as plain Git patches:

```sh
jj git export
git format-patch main..<stack-head> -o /home/ankit/hroot/projects/dockers/bifrost-dynamic/patches
```

The Dockerfile should remain independent of `jj`.

## Commands

```sh
just patches-list
just patches-check
```

These commands only inspect repository files. They do not build images, restart
services, or modify running containers.
