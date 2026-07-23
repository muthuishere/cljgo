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
