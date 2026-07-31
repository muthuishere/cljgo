# s45 comparison corpus — "hello + JSON" HTTP servers

Equivalent minimal HTTP servers across six runtimes, each Dockerized, so the
s45 orchestrator can benchmark them **serially** (one machine, one load test at
a time) against a compiled `cljgo`/`bri.http` server. This directory only
**builds, smoke-tests, and records image sizes** — it deliberately runs **no**
comparative load benchmark (that must be serial and is the orchestrator's job).

## Contract (every server behaves identically)

Listens on `$PORT` (default `8080`), exposes exactly two GET routes:

| route         | status | Content-Type       | body                                   |
|---------------|--------|--------------------|----------------------------------------|
| `GET /`       | 200    | `text/plain`       | `hello\n`                              |
| `GET /api/hello` | 200 | `application/json` | `{"msg":"hello from <runtime>"}` (no trailing newline/whitespace) |

`<runtime>` ∈ {`go`, `ring-jetty`, `http-kit`, `bun`, `node`, `deno`}.

## Results (measured 2026-07-24, docker 29 / OrbStack, arm64)

| runtime    | image size | both-routes smoke | base image                        | dir              |
|------------|-----------:|:-----------------:|-----------------------------------|------------------|
| go         |   7.62 MB  | yes               | `scratch` (from `golang:1.26-alpine` build) | `go/`            |
| node       |    228 MB  | yes               | `node:24-alpine`                  | `node/`          |
| deno       |    277 MB  | yes               | `denoland/deno:latest`            | `deno/`          |
| bun        |    333 MB  | yes               | `oven/bun:1`                      | `bun/`           |
| http-kit   |    847 MB  | yes               | `clojure:temurin-26-tools-deps`   | `clj-httpkit/`   |
| ring-jetty |    858 MB  | yes               | `clojure:temurin-26-tools-deps`   | `clj-ring-jetty/`|

All six build cleanly and pass the exact status / Content-Type / body assertion
on both routes.

## How to build / run one

```bash
cd <dir>
docker build -t s45-<runtime> .
docker run -d -p 8080:8080 --name s45-<runtime> s45-<runtime>
curl -i localhost:8080/            # hello\n, text/plain
curl -i localhost:8080/api/hello   # {"msg":"hello from <runtime>"}, application/json
docker rm -f s45-<runtime>
```

## Rebuild + smoke-test all six

```bash
./smoke.sh
```

`smoke.sh` is idempotent and serial: per runtime it removes any prior
container, rebuilds the image, runs it on a distinct host port (8091..8096),
waits for readiness, curls both routes and asserts exact status/Content-Type/
body, records the image size, then `docker rm -f`s the container. It never
leaves a container running and never runs two load tests at once (it runs no
load test at all). Exit code 0 = all six passed.

## Design notes per runtime

- **go** — `net/http` with an explicit `ServeMux`. Built `CGO_ENABLED=0`,
  `-ldflags="-s -w"`, final stage `scratch`. This is the native ceiling and the
  smallest image by two orders of magnitude. No TLS/certs needed (plain HTTP).
- **node** — `node:http` `http.createServer`, no dependencies, `node:24-alpine`.
- **deno** — `Deno.serve(...)`, run with `--allow-net --allow-env`.
- **bun** — `Bun.serve({...})` on `oven/bun:1` (Debian-based, hence larger than
  the alpine node image).
- **ring-jetty** — Clojure + `ring/ring-jetty-adapter` 1.14.2, run via
  `clojure -M -m app.core` (no uberjar — simplest reliable boot). Deps are
  prefetched at build time (`clojure -P`) and the `~/.m2` cache is copied into
  the runtime stage so the container needs no network at boot.
- **http-kit** — Clojure + `http-kit` 2.8.0, same packaging as ring-jetty.
  `-main` parks on `@(promise)` to keep the process alive (http-kit's
  `run-server` returns immediately, unlike Jetty's `:join? true`).

## Gotchas / honest caveats

- **JVM images are ~850 MB** — the full `clojure:temurin-26-tools-deps` base
  (JDK + tools.deps + the whole `~/.m2` dependency cache) dominates. This is a
  real data point for the s45 comparison, not a build failure. Slimming is
  possible (uberjar onto a JRE-only or distroless base, jlink custom runtime)
  but was deliberately not done — the corpus favors the *simplest reliable
  boot* and honest defaults over hand-tuned images. Expect the JVM servers to
  also carry a **cold-start** penalty (JVM boot + Clojure runtime init +
  first-request JIT) that the orchestrator's serial benchmark should account for
  (warm-up requests before measuring).
- **Content-Type charset** — Jetty/Ring appends `; charset=UTF-8` to the
  header. `smoke.sh` compares the media type only (strips `;...`), and the
  contract is satisfied (`text/plain` / `application/json`). The orchestrator
  should likewise compare the media type, not the raw header string, if it
  re-asserts.
- **JSON body is byte-exact** — no pretty-printing, no trailing newline. Each
  server writes the literal string, so `{"msg":"hello from node"}` etc. match
  exactly.
- **go image = 7.62 MB** because `scratch` + static binary; it carries no shell,
  so debug via `docker logs` / rebuild, not `docker exec`.
- Base image tags are pinned to major where practical (`node:24-alpine`,
  `golang:1.26-alpine`, `clojure:temurin-26-tools-deps`, `oven/bun:1`) but
  `denoland/deno:latest` floats — pin it if reproducibility matters to the run.

## Additional languages (measured 2026-07-24, docker 29 / OrbStack, arm64)

Four more runtimes added on top of the original six, same contract exactly
(`GET /` → `hello\n` text/plain; `GET /api/hello` →
`{"msg":"hello from <runtime>"}` application/json, byte-exact). Built and
smoke-tested identically to the six above, on distinct host ports (8101..8104)
via `smoke-extra.sh` so they never collide with `smoke.sh`. The existing dirs
were not touched.

| runtime     | image size | smoke pass | base image                                    | notes |
|-------------|-----------:|:----------:|-----------------------------------------------|-------|
| rust-axum   |   140 MB   | yes        | `debian:stable-slim` (from `rust:1-slim` build) | axum 0.7 + tokio; explicit `Content-Type` headers; the top-performance reference. LTO + `strip` release build. Final image is `debian-slim` (not scratch/musl) for the simplest reliable dynamic-linked build. |
| fastapi     |   220 MB   | yes        | `python:3.13-slim`                            | uvicorn, **workers=1** for a fair single-process baseline. Both routes return a hand-built `Response(content=..., media_type=...)` so the bytes are exact (FastAPI never re-serializes / reorders / whitespaces the JSON). |
| dotnet      |   359 MB   | yes        | `mcr.microsoft.com/dotnet/aspnet:9.0` (from `sdk:9.0` build) | ASP.NET Core minimal API, `net9.0`. `Results.Text(literal, media-type)` for exact bodies. Built cleanly first try — did not fight. |
| spring-boot |   512 MB   | yes        | `eclipse-temurin:21-jre` (from `maven:3.9-eclipse-temurin-21` build) | Spring Boot 3.4.1 `@RestController`, embedded Tomcat, fat jar. `produces=` sets the media types; returns the exact `String`. Largest image + JVM cold-start (expected) — the orchestrator should warm up before measuring. `PORT` mapped via `server.port=${PORT:8080}`. |

All four build cleanly and pass the exact status / Content-Type / body
assertion on both routes.

### Notes / honest caveats (additional languages)

- **fastapi single-process** — deliberately `uvicorn ... workers=1`, matching
  the other single-process baselines here (node/deno/bun/go are one process).
  A production FastAPI deploy would run multiple workers; that would be a
  different, unfair comparison, so it's pinned to one on purpose.
- **spring-boot cold start** — JVM boot + Spring context init + first-request
  JIT. The 512 MB image is the temurin JRE + fat jar (Tomcat, Spring, Jackson).
  Both the size and the cold-start are the expected data point, not a defect.
  Slimming (jlink / distroless / layered jar) was intentionally skipped in
  favor of the simplest reliable boot, consistent with the JVM Clojure servers.
- **rust-axum image = 140 MB** — dominated by `debian:stable-slim`; the binary
  itself is a few MB. A `scratch` + musl-static final stage would cut this
  dramatically but the dynamic debian build is the simplest reliable path
  (glibc, no musl target juggling). Pin `rust:1-slim` if reproducibility
  matters.
- **dotnet** — built without a fight; kept on the standard `aspnet:9.0` runtime
  (not distroless/AOT) for the simplest reliable boot. `InvariantGlobalization`
  is on to avoid ICU bloat.
- **Content-Type charset** — as with the JVM servers, some of these frameworks
  may append `; charset=...`; `smoke-extra.sh` compares the media type only
  (strips `;...`), exactly like `smoke.sh`.

### Rebuild + smoke-test just the additional four

```bash
./smoke-extra.sh
```

Same idempotent, serial behavior as `smoke.sh` (ports 8101..8104, never runs
two at once, never leaves a container running, runs no load test). Exit code
0 = all four passed.
