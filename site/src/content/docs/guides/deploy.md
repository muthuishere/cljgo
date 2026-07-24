---
title: "Deploy: one static binary"
description: "A bri app AOT-compiles to a single CGO_ENABLED=0 binary and deploys as a ~15 MB scratch Docker image — the dev loop is a REPL, the artifact is one file."
---

A bri app AOT-compiles to a single static `CGO_ENABLED=0` binary, byte-identical to the interpreter path (ADR 0071). The dev loop is a REPL (`cljgo dev`, live re-`def`, nREPL); the deploy artifact is one file with no runtime to distribute — the design bet the [benchmarks](/cljgo/reference/benchmarks/) measure (~15 MB image, ~30 ms cold-start, ~16 MB RSS).

## Build a binary

```bash
cljgo build -o server src/app/main.cljg     # AOT-compile to one static binary
CGO_ENABLED=0 GOOS=linux cljgo build -o server src/app/main.cljg   # for a Linux image
cljgo dist --target linux/amd64,linux/arm64  # or cross-compile a whole matrix at once (ADR 0077)
```

The binary links the **compiled** core (never the interpreter), so it starts in single-digit milliseconds. It embeds nothing it doesn't need — an app that never requires `bri.otel` never links the OpenTelemetry SDK (ADR 0074), and a db-less app never links SQLite/pgx (ADR 0076). For shipping binaries to many platforms at once (releases, Homebrew), see [`cljgo dist`](/cljgo/guides/compile/).

## The Dockerfile

`cljgo new --template web` ships a multi-stage `Dockerfile` that AOT-compiles the app to a static binary and copies it into a `scratch` image — typically single-digit MB plus your config and static assets:

```dockerfile
# syntax=docker/dockerfile:1
ARG CLJGO_VERSION=v0.6.0

FROM golang:1.26 AS build
ARG CLJGO_VERSION
RUN go install github.com/muthuishere/cljgo/cmd/cljgo@${CLJGO_VERSION}
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux cljgo build -o /server src/app/main.cljg

FROM scratch
COPY --from=build /server /server
COPY --from=build /app/conf.edn /conf.edn
COPY --from=build /app/public /public
ENV APP_PROFILE=prod
EXPOSE 3000
ENTRYPOINT ["/server"]
```

The build stage `go install`s the cljgo compiler at a pinned `CLJGO_VERSION` and fetches the matching runtime from the Go module proxy — no repo checkout. Bump `CLJGO_VERSION` to move to a newer cljgo.

```bash
docker build -t app .
docker run -p 3000:3000 app
```

## Config and secrets in the image

`conf.edn` defaults `:port` to 3000; any key is overridden by an `APP_*` env var (the secrets-are-env doctrine), so the same image runs in every environment — see [bri.config](/cljgo/bri/config/):

```bash
docker run -p 3000:3000 \
  -e APP_PROFILE=prod \
  -e APP_DB_URL=postgres://… \
  -e APP_AUTH__SECRET=… \
  app
```

Secrets never live in the image — they arrive as env at run time. `APP_DB_URL` flips [bri.db](/cljgo/bri/db/) from the zero-install SQLite default to Postgres with no code change; `APP_AUTH__SECRET` is the [bri.auth](/cljgo/bri/auth/) signing key that prod must set.

## Migrations on deploy

`(db/migrate! conn "migrations")` is idempotent and forward-only — run it on boot (the `delay` pattern) or as a one-shot before rollout. It is the same call in dev and prod; running it twice is a no-op. See [bri.db](/cljgo/bri/db/).

## Ops endpoints

The API stack serves ops endpoints by default: `GET /healthz` (liveness), `GET /readyz` (runs `:ready-checks` → 200/503), and `GET /metrics` (Prometheus text) — wire them straight into your orchestrator's probes. Guard `/metrics` in prod with `:metrics-guard (auth/admin-only)`.

## Where next

- [Compile & ship binaries](/cljgo/guides/compile/) — the general `cljgo build` story
- [bri.config](/cljgo/bri/config/) — profiles, `APP_*` env, and the schema
- [bri.db](/cljgo/bri/db/) — SQLite by default, Postgres via `APP_DB_URL`
- [Benchmarks](/cljgo/reference/benchmarks/) — the measured image / cold-start / RSS figures
