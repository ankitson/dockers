# dockers

Small docker image build contexts that don't belong inside the repo of any
specific consumer. Each subdirectory is one image — `Dockerfile`, an entrypoint
or two, anything else the image needs at build time.

## Images

- `openclaw/` — thin layer over `ankit/devbox` that installs the openclaw
  gateway + ACP harnesses. Consumed by `~/hroot/devserver/docker-compose.yml`
  (service `openclaw`).

## Adding a new image

1. Drop a folder `<name>/` with a `Dockerfile` (+ entrypoints, configs).
2. In whichever `docker-compose.*.yml` consumes it, point `build.context` at
   `/projects/dockers/<name>` (absolute path keeps it independent of the
   consumer's cwd).
3. Build + recreate: `cd ~/hroot/<consumer> && just build <service> && just up -d <service>`.

## Why a separate repo?

These images don't have a natural home in any of the consumer repos
(devserver, homeserver, etc.) and they're not big enough to deserve their own
repo each. Single shared place makes them git-versioned, easy to share across
machines, and avoids "which compose file owns this Dockerfile?" confusion.
