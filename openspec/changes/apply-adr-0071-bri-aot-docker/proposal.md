# apply-adr-0071-bri-aot-docker

## Why

ADR 0071 (docs/adr/0071-bri-aot-static-binary-docker.md, proposed; spike s45
gates it) makes bri — the flagship web framework — deployable the way cljgo
deploys everything else: a single static `CGO_ENABLED=0` binary in a minimal
Docker image. Today `cljgo build` on a bri app fails (`could not locate
namespace bri.http`) because the `pkg/bri` host shims and `core/bri/*.cljg`
sources are registered as **interpreter** lib providers only; the emitter has
no bri provider and `pkg/bri` is not linked into a user binary. bri runs
interpreted (`cljgo dev`) but cannot compile.

Owner mandate: bri must run **both** interpreted (dev/nREPL) **and** compiled
(prod/Docker), byte-identical, and *"if a JVM Clojure web stack is faster than
this, there is no point"* — so the same flagship (bri.http, not a raw net/http
proxy) must be proven faster and leaner than JVM Clojure web in a real Docker
image before rollout.

Feasibility is proven: `clojure.core.async` — same shape (`.cljg` source + Go
shims + lib provider) — already AOT-compiles to a 6.7 MB static binary. bri
differs only in that its shims live in `pkg/bri` (not the always-linked
`pkg/corelib`) and its registration runs solely in the REPL driver. So this is
"follow the async/coreaot pattern" (ADR 0042 emitted-package lib providers, ADR
0046 AOT core), not new emitter capability.

## What Changes

- **bri emits + links in the compiled path.** Give each bri namespace an
  emitted-package lib provider (ADR 0042 §2): the `core/bri/*.cljg` sources
  (already embedded via `core/bri.go`) resolve and emit at build like
  `core/async.cljg`; the `pkg/bri` host shims (`-serve` `-request` `-json-*`
  `-jwt-sign` `-argon2-*` `-hmac-sign` `-rand-token` … all pure-Go: `net/http`
  + `x/crypto`, no cgo) register in the **compiled** binary so `require
  'bri.http` resolves at build and the binary links them. The generated go.mod
  requires the bri runtime package when the app uses bri.
- **`func main()` dispatches to the app `-main`** so a compiled bri app is a
  server that calls `http/serve`/`http/listen` and blocks on SIGTERM — not a
  REPL. Native `net/http` + emitted-Go handlers over the existing `lang.IFn`
  seam.
- **Dual-mode parity harness.** A bri behavior suite runs the same app
  interpreted AND compiled and asserts byte-identical responses (status,
  headers, body, JWT round-trip, guard 401→200, funnel codes). A REPL↔binary
  divergence fails the build (CLAUDE.md).
- **Docker as a first-class artifact.** `templates/web` gains a multi-stage
  `Dockerfile` (Go-toolchain build stage running `cljgo build` → `scratch`/
  distroless final with only the static binary) + `.dockerignore`.
- **The perf guarantee (spike s45).** A compiled bri.http hello-world (+ one
  JSON route) is benchmarked on one machine, serially, vs Go `net/http`, JVM
  Clojure (Ring+Jetty and http-kit), Bun, Node, Deno: req/s, p50/p99, **peak
  RSS**, **Docker image size**, cold-start. VERDICT gates ADR 0071 accept/stop;
  the numbers, once green, are CI-tracked (design/00 §1.4) and published.

## Non-goals

- Rewriting bri's HTTP server — it is already native Go (`pkg/bri/http.go`);
  only the wiring/emit path changes.
- Dropping the interpreter path — interpreted bri stays the dev loop; both
  modes ship (owner: "both compiled and interpreted").
- bri.db / batteries (ADRs 0057–0062) — out of scope; this change is the
  deploy + perf floor the flagship API tier stands on.
- A `require-go '["net/http"]` emit fix — bri uses its own `pkg/bri` server,
  so the (separate, real) multi-namespace require-go emit bug does not block
  this and is tracked elsewhere.
