# bri-deployment Specification

## Purpose
TBD - created by archiving change apply-adr-0071-bri-aot-docker. Update Purpose after archive.
## Requirements
### Requirement: bri apps AOT-compile to a single static binary
`cljgo build` on a project whose sources `(require '[bri.http …])` (and the
other bri namespaces: `bri.auth`, `bri.audit`, `bri.config`, `bri.html`) SHALL
produce a single native binary with `CGO_ENABLED=0`, resolving and linking bri
at build time — no "could not locate namespace bri.http" error, no interpreter
in the binary. The bri host shims SHALL be pure-Go (net/http + x/crypto) so the
static-binary constraint holds.

#### Scenario: a bri hello-world compiles and serves
- **WHEN** a project defines `GET /` (text) and `GET /api/hello` (JSON) with
  bri.http and is built with `cljgo build`
- **THEN** the build succeeds, the binary starts an HTTP server, and both
  routes return their expected status/content-type/body

#### Scenario: the binary is statically linked
- **WHEN** the compiled bri binary is inspected (`ldd` / `otool -L`)
- **THEN** it has no dynamic dependency that would break a `scratch` image
  (CGO_ENABLED=0 honored)

### Requirement: compiled bri invokes the app entrypoint
A compiled bri application's `func main()` SHALL invoke the application's
`-main`, which starts the server (`bri.http/serve` or `/listen`) and blocks
until SIGTERM/SIGINT then drains — so the binary is a runnable server, not a
REPL, and the emitted request path uses native `net/http` with emitted-Go
handlers (no per-request tree-walk).

#### Scenario: the built binary runs the server
- **WHEN** the compiled binary is executed
- **THEN** it listens on the configured port and serves requests until
  signalled, then drains in-flight requests

### Requirement: interpreted and compiled bri are byte-identical
The same bri application served interpreted (`cljgo dev`) and compiled SHALL
return byte-identical responses — status line, headers, and body — for a
representative surface: a text route, a JSON route, a JWT-guarded route
(401 without a token, 200 with a valid token), and an error-funnel case
(e.g. a bad path param → 400). A divergence between the two modes SHALL fail
the build (dual-harness discipline; a REPL↔binary divergence is a release
blocker).

#### Scenario: guarded route agrees across modes
- **WHEN** a JWT-guarded route is exercised with no token then a valid token,
  in both the interpreted and the compiled app
- **THEN** both modes return 401 then 200 with identical bodies and headers

#### Scenario: JSON + funnel agree across modes
- **WHEN** a JSON route and a bad-path-param route are exercised in both modes
- **THEN** the JSON bodies are byte-identical and both return the same funnel
  status (e.g. 400) with the same body

### Requirement: the web template ships a Docker deployment
`templates/web` (what `cljgo new --template web` generates) SHALL include a
multi-stage `Dockerfile` and a `.dockerignore` that build the app with the Go
toolchain (`cljgo build`) and produce a minimal final image (`scratch` or
distroless) containing only the static binary, running the server. The image
SHALL start and serve the generated app's routes.

#### Scenario: docker build yields a serving image
- **WHEN** `docker build` is run in a generated web project
- **THEN** it produces an image that, when run, serves the app's `/` and
  ops (`/healthz`) routes

### Requirement: compiled bri.http meets the performance guarantee (spike s45)
A compiled bri.http hello-world (+ one JSON route) SHALL be benchmarked on a
single machine, serially (no concurrent runs — contention skews results),
against Go `net/http`, JVM Clojure (Ring+Jetty and http-kit), Bun, Node, and
Deno, capturing req/s, p50/p99 latency, peak RSS, Docker image size, and
cold-start. The result SHALL be recorded in spike s45's VERDICT. The guarantee
to ratify ADR 0071 is that compiled bri.http is clearly faster and leaner than
JVM Clojure web; if it is not, ADR 0071 does not proceed.

#### Scenario: the flagship, not a proxy
- **WHEN** the benchmark is run
- **THEN** the cljgo entrant is bri.http itself (not a raw net/http server),
  compiled, in its Docker image, measured on the same routes and payloads as
  every other runtime

