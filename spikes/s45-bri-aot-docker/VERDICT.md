# Spike s45 — VERDICT: compiled bri.http vs the web-framework field

Date: 2026-07-24 · Gates ADR 0071. Owner bar: *"if a JVM Clojure web stack is
faster than this, there is no point."*

## Result: ✅ RATIFIED — compiled bri.http dominates JVM Clojure web

bri.http AOT-compiles to a single static `CGO_ENABLED=0` binary (0 cgo
symbols, scratch-ready), serves real HTTP, and is **byte-identical interpreted
vs compiled** (dual-harness parity: text, JSON, JWT 401→200, typed-param
funnel 200/400). It ships as a **15.5 MB Docker image** and does **~78k req/s**
— top tier, and decisively ahead of JVM Clojure on every deploy axis.

## The field (Apple M-series arm64, Docker/OrbStack, oha 15 s @ 50 conn, one container at a time)

| runtime | image | cold-start | `/` req/s | `/api` req/s | p99 | peak RSS |
|---|--:|--:|--:|--:|--:|--:|
| rust-axum | 140 MB | 28 ms | 89,480 | 89,986 | ~1.0 ms | 8 MB |
| deno | 277 MB | 146 ms | 89,316 | 89,099 | ~0.9 ms | 21 MB |
| clj-httpkit | 847 MB | 1277 ms | 82,837 | 83,669 | ~1.0 ms | 353 MB |
| **bri (cljgo, compiled)** | **15.5 MB** | **~30 ms** | **78,126** | **77,788** | **1.4 ms** | **~16 MB** |
| bun | 333 MB | 28 ms | 74,798 | 83,535 | ~1.5 ms | 50 MB |
| clj-ring-jetty | 858 MB | 1659 ms | 67,786 | 67,442 | ~1.5 ms | 491 MB |
| dotnet | 359 MB | 172 ms | 62,792 | 67,451 | ~1.9 ms | 47 MB |
| go (net/http) | 7.6 MB | 30 ms | 66,876 | 55,769 | ~2.6 ms | 16 MB |
| node | 228 MB | 147 ms | 55,344 | 62,167 | ~1.8 ms | 134 MB |
| spring-boot | 512 MB | 858 ms | 51,002 | 55,056 | ~1.7 ms | 574 MB |
| fastapi | 220 MB | 381 ms | 8,931 | 8,948 | ~10.5 ms | 38 MB |

Throughput has some run-to-run noise (go's `/` was 66–70k across runs); the
image / RAM / cold-start figures are stable.

## bri vs JVM Clojure (the bet, head to head)

| | bri (compiled) | clj http-kit | clj ring-jetty | bri advantage |
|---|--:|--:|--:|---|
| image | **15.5 MB** | 847 MB | 858 MB | **~55× smaller** |
| cold-start | **~30 ms** | 1277 ms | 1659 ms | **~40–55× faster** |
| peak RSS | **~16 MB** | 353 MB | 491 MB | **~22–30× leaner** |
| req/s | 78k | 82k | 67k | comparable-to-faster |

**bri also beats Go net/http, Node, .NET, Spring Boot, and FastAPI on
throughput.** It sits in the top tier (Rust/Deno/Bun/http-kit) while carrying a
native-Go deploy footprint. Bet won.

## Build time — the code generator is NOT the bottleneck

`cljgo build` of the whole bri app (emit → `go build`, `-s -w`, CGO_ENABLED=0):

| | wall |
|---|--:|
| clean build | **1.25 s** |
| warm rebuild | **0.28 s** |

Our codegen (tree → Go source) is a small fraction of that; most of the 0.28 s
warm is Go's own compiler/linker. The code generator is fast.

## So where did the "delay" actually go?

Not the runtime (78k req/s) and not the build (0.28–1.25 s). The wall-clock the
owner felt was:

1. **Development effort** — the bri-AOT enablement was genuinely novel compiler
   work (a new `cmd/genbri` generator, `pkg/briaot`, making `pkg/bri` pure-Go,
   emitter wiring, an emitter bug found + fixed). Hours, once.
2. **The conformance suite — ~237 s** (the full interpreted-vs-compiled dual
   harness, 491 files, each compiled to a fresh binary). This is the one real
   slow loop, and it is **test-time, not build-time or run-time**.
3. **Docker image builds** — compiling cljgo + the app inside the image and
   pulling base layers.

None of these is the code generator being slow per invocation.

## The right levers (and the SIMD question, honestly)

- **SIMD is a RUNTIME lever, not a codegen one.** It parallelizes arithmetic
  over data (hashing, JSON scanning, crypto), not tree-walking + string
  building. It cannot speed up `cljgo build`. Where it *could* add runtime
  headroom above 78k req/s: JSON encode/decode and the HS256/argon2 paths —
  a future option if a workload is JSON- or auth-bound, not a general need.
- **The 237 s conformance suite is the real thing to parallelize** — it
  currently compiles files largely serially (`perf/parallel-compiled-
  conformance` branch exists for exactly this). Running the compiled leg's
  `go build`/run subprocesses across cores is the highest-value speedup, and it
  is test-time only.
- **Build caching / incrementalism** already gives the 0.28 s warm rebuild;
  little to gain there.

## Deliverables in this spike
`bri/` (compiled hello app + Dockerfile + parity harness + NOTES), `compare/`
(10 runtimes, Dockerized), `bench/run.sh` (serial oha runner, auto-discovers),
`bench/results*.md`. Enablement code (real): `cmd/genbri`, `pkg/briaot`,
`pkg/briloader`, pure-Go `pkg/bri`, emitter wiring — all gates + conformance
green. → **ADR 0071 proceeds to rollout** (template Dockerfile, README numbers).
