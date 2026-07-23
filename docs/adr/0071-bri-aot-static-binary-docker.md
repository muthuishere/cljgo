# ADR 0071 — bri apps AOT-compile to a single static binary, deployable as a minimal Docker image
Date: 2026-07-24 · Status: accepted (spike s45 VERDICT ratified — compiled
bri.http ~78k req/s / 15.5 MB / ~30 ms cold-start dominates JVM Clojure web:
~55× smaller image, ~40–55× faster start, ~22–30× less memory; dual-mode
parity byte-identical, full conformance green. Shipped in v0.4.0.)

## Context
cljgo's identity is a **single, static, CGO_ENABLED=0 native binary** — ~5 MB,
~5 ms startup, no runtime install. bri (ADR 0041/0069) is the flagship web
framework built on that. The intended deployment is a **minimal Docker image**
(scratch/distroless holding just the binary); for a web API, the container IS
the artifact, and the owner's bar is blunt: *if a JVM Clojure web stack is
faster than this, there is no point.*

Today a bri app **cannot AOT-compile**. `cljgo build` on `examples/web-api`:

```
error: could not locate namespace bri.http (no registered provider, and no
bri/http.clj/.cljg/.cljc relative to the requiring file)
```

The reason is NOT the server. bri's HTTP server is already **native compiled
Go** (`pkg/bri/http.go`: `net/http`, Go 1.22+ `ServeMux`, `http.Server`); a
request hits a real `http.HandlerFunc` that invokes the handler through
`lang.Apply(ifn, …)` — the *same* `lang.IFn` seam the emitter produces. What is
interpreter-only is the **wiring**: `pkg/bri.Register` (called by the REPL
driver) registers the `-serve`/`-request`/`-jwt-sign`/… host shims and the
`core/bri/*.cljg` framework sources as **interpreter** lib providers at
runtime. The emitter has no bri provider, and `pkg/bri` is not linked into a
user binary, so a build cannot resolve or link bri.

Feasibility is **proven**: `clojure.core.async` — the same shape (`.cljg`
source `core/async.cljg` + Go shims + a lib provider) — AOT-compiles and runs
in a 6.7 MB static binary today. The difference is only that async's shims live
in the always-linked `pkg/corelib`, while bri's live in a separate `pkg/bri`
registered solely by the interpreter. So this is "**follow the async/coreaot
pattern**" (ADR 0042 emitted-package lib providers, ADR 0046 AOT core), not new
emitter capability.

## Decision
1. **A bri app MUST AOT-compile to one static CGO_ENABLED=0 binary**, exactly
   like any cljgo program. This is the blessed deploy path; the interpreter
   (`cljgo dev`) is for development only. The template `build.cljgo`'s "AOT of
   bri apps lands with a later tier" note is superseded — this ADR is that
   tier.
2. **Emit + link bri through the compiled path.** The `core/bri/*.cljg`
   namespaces resolve and emit at build (they are already embedded via
   `core/bri.go`); the `pkg/bri` host shims (all pure-Go — `net/http`,
   `x/crypto` argon2/HS256, no cgo) register in the **compiled** binary via an
   emitted-package lib provider (ADR 0042 §2), so `require 'bri.http` resolves
   at build and the binary links the shims. The generated `go.mod` requires
   `pkg/bri` when the app uses it.
3. **`func main()` invokes the app's `-main`** (which calls `http/serve` /
   `http/listen`) — a compiled bri app is a server binary, not a REPL. Native
   `net/http` + emitted-Go handlers over the same `lang.IFn` seam ⇒ the AOT
   ceiling ≈ Go `net/http`; the per-request tree-walk is gone.
4. **Ship Docker as a first-class artifact.** `templates/web` gains a
   multi-stage `Dockerfile` (Go-toolchain build stage running `cljgo build` →
   `scratch`/distroless final holding only the static binary) and a
   `.dockerignore`. `cljgo build` is the whole build step.
5. **The flagship benchmark is bri.http itself** (owner: "test only with
   bri.http, that's our flagship") — NOT a raw `net/http` proxy. A compiled
   bri.http hello-world (+ a JSON route) is measured for req/s, p99 latency,
   **peak RSS**, **Docker image size**, and cold-start against Go `net/http`,
   JVM Clojure (Ring+Jetty and http-kit), Bun, Node, and Deno on one machine.
   This is spike **s45**, and its VERDICT gates full rollout: if compiled
   bri.http is not clearly faster/leaner than JVM Clojure web, we stop and
   rethink before investing further.

6. **Both modes, identical behavior — not compiled INSTEAD of interpreted**
   (owner: "all bri should be both compiled and interpreted for high
   performance"). Interpreted bri stays the dev loop (`cljgo dev`, live
   re-`def`, nREPL); compiled bri is production. Neither is dropped, and the
   two MUST agree: bri joins the **dual-harness** discipline (CLAUDE.md — a
   REPL↔binary divergence is a release blocker), so a bri behavior suite runs
   the same app **interpreted AND compiled** and asserts byte-identical
   responses (status, headers, body, JWT round-trip, guard 401/403, funnel
   codes). The `pkg/bri` shims are the single Go implementation both modes
   share, which is what makes parity structural rather than hoped-for.

## Consequences
A bri web API deploys as a single-digit-MB scratch image with ms-scale startup
and native-Go request throughput — the "Bun of Clojure" claim becomes a
measured Docker artifact, not an aspiration. Work is bounded: a `pkg/briaot`
(or an extension of the coreaot generate step) emits the bri sources; a
compiled-path registration links the existing pure-Go shims; `func main`
dispatches to `-main`; a Dockerfile lands in the template. All shims staying
pure-Go keeps CGO_ENABLED=0 intact (the sacred constraint). Risk: if the
per-request path retains hidden interpreter dependencies (e.g. a shim that
reaches back into `eval`), those must be severed — the spike surfaces them
first. Benchmarks (image size / peak RSS / req-s) become CI-trackable once the
binary exists (perf-is-a-feature, design/00 §1.4).
