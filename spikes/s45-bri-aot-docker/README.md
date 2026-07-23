# Spike s45 — bri.http AOT to a static binary in Docker, benchmarked vs the field

Gates **ADR 0071**. Owner mandate: *"if a JVM Clojure web stack is faster than
this, there is no point"* — so prove a **compiled** bri.http web API is clearly
faster and leaner than JVM Clojure web, in a real Docker image, before we
invest further. Flagship-only: measure **bri.http itself**, not a raw
`net/http` proxy.

## The question
Does a bri.http hello-world (+ one JSON route), AOT-compiled to a single static
`CGO_ENABLED=0` binary and shipped in a minimal Docker image, beat JVM Clojure
web (and hold its own vs Go / Bun / Node / Deno) on throughput, tail latency,
peak memory, image size, and cold start?

## Exit criteria (ADR 0027)
1. **bri.http compiles.** `cljgo build` turns a bri hello-world (`GET /` text +
   `GET /api/hello` JSON) into a running static binary that serves real HTTP —
   the ADR 0071 enablement (emit `core/bri/*.cljg`, link `pkg/bri` shims in the
   compiled path, `func main` → app `-main`). `CGO_ENABLED=0`, `otool -L` /
   `ldd` shows no dynamic libs beyond libc-none (scratch-ready).
2. **Dual-mode parity.** The same app served interpreted (`cljgo dev`) and
   compiled returns byte-identical responses (status/headers/body) for `/`,
   `/api/hello`, and a JWT-guarded route (401 → 200). A REPL↔binary divergence
   fails the spike (CLAUDE.md).
3. **Docker.** A multi-stage `Dockerfile` (Go-toolchain build → `scratch` or
   distroless final) yields an image running the binary. Record its size.
4. **Comparison corpus** under `compare/`: equivalent hello + JSON servers,
   each Dockerized —
   - `go` — `net/http` (the native ceiling)
   - `clj-ring-jetty` — Ring + Jetty (`clojure` CLI, JVM 26)
   - `clj-httpkit` — http-kit
   - `bun` — `Bun.serve`
   - `node` — `node:http`
   - `deno` — `Deno.serve`
5. **One-machine, serial benchmark** (the orchestrator runs it, never in
   parallel — contention skews numbers): a warmed load test (`oha`/`hey`, fixed
   duration + concurrency) capturing **req/s, p50/p99, peak RSS, image size,
   cold-start ms** for every runtime, on `/` and `/api/hello`. Identical
   payloads; results table committed.

## VERDICT (written on completion → feeds ADR 0071 accept/stop)
`VERDICT.md`: the table + a one-line call — is compiled bri.http clearly ahead
of JVM Clojure web? If yes, ADR 0071 proceeds to full rollout (dual-harness,
template Dockerfile, README numbers). If no, stop and rethink.

## Layout
```
s45-bri-aot-docker/
  README.md            this (exit criteria)
  bri/                 the bri hello app (src + Dockerfile) — fills once compile works
  compare/             go · clj-ring-jetty · clj-httpkit · bun · node · deno (+ Dockerfiles)
  bench/               the serial runner + results table
  VERDICT.md           written last
```
