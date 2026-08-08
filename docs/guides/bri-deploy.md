# Deploy — one static binary

A bri app AOT-compiles to a single static `CGO_ENABLED=0` binary,
byte-identical to the interpreter path (ADR 0071). The dev loop is a REPL
(`cljgo dev`); the artifact is one file with no runtime to distribute
(~20 MB image, ~30 ms cold-start, ~35 MB RSS under load — measured
2026-08-02, see the
[benchmarks page](https://muthuishere.github.io/cljgo/reference/benchmarks/)).

Full guide on the site: https://muthuishere.github.io/cljgo/guides/deploy/

## Build

```bash
cljgo build -o server src/app/main.cljg
CGO_ENABLED=0 GOOS=linux cljgo build -o server src/app/main.cljg   # Linux image
```

Links the compiled core (not the interpreter) → single-digit-ms start.
Conditional linking (ADR 0074): an app that never requires `bri.core.telemetry`
never links the OTel SDK.

## Dockerfile

`cljgo new --template web` ships `templates/web/Dockerfile` — a multi-stage
build that `go install`s the cljgo compiler at a pinned `CLJGO_VERSION`
(default `v0.9.0`), AOT-compiles, and copies the binary into a `scratch`
image with config + static assets:

```dockerfile
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

```bash
docker build -t app .
docker run -p 3000:3000 app
```

## Config & secrets

`conf.edn` defaults `:port` 3000; any key is overridden by an `APP_*` env
var (secrets-are-env) — the same image runs everywhere. `APP_DB_URL` flips
cljg.data.cast to Postgres with no code change; `APP_AUTH__SECRET` is the JWT key
prod must set.

```bash
docker run -p 3000:3000 -e APP_PROFILE=prod -e APP_DB_URL=postgres://… \
  -e APP_AUTH__SECRET=… app
```

## Migrations & ops

`(db/migrate! conn "migrations")` is idempotent + forward-only — run on
boot (the delay pattern) or one-shot before rollout. The API stack serves
`GET /healthz`, `GET /readyz` (`:ready-checks` → 200/503) by default — wire into probes.

`GET /metrics` is **closed unless you configure it** (ADR 0126), because it
exposes per-route traffic and latency. Pick one:

- `APP_METRICS_TOKEN=…` — served behind `Authorization: Bearer <token>`.
- `:metrics-guard (auth/admin-only)` — your own gate.
- `:metrics-guard :public` — open, for a private network or sidecar scrape.

With none of them set, the route is not mounted outside dev and a line on
stdout tells you so. A miss returns 404, not 401, so a scan cannot confirm
the endpoint exists.

## See also

- `docs/guides/bri-db.md` · `docs/guides/bri-config.md`
- the [benchmarks page](https://muthuishere.github.io/cljgo/reference/benchmarks/) — the measured web-framework benchmark table
