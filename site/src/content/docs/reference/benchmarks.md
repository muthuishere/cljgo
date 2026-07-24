---
title: Benchmarks
description: Measured, reproducible numbers — boot time, memory, binary size, and a head-to-head against let-go, babashka, joker, and JVM Clojure. Wins and losses both published.
---

Performance in cljgo is a gated feature, not a marketing claim: perf budgets
run inside plain `go test ./...` and a regression is treated like a
conformance failure. Every number on this page was **measured, not quoted** —
and the rows cljgo loses are published alongside the ones it wins.

**Measurement context for everything below:** Apple M5 Pro, go1.26.3, cljgo
@HEAD (post-ADR-0067), re-measured **2026-07-23**. `hello.clj` =
`(println "hi")`. Every competing runtime was installed and measured on the
same machine — no normalization, no numbers copied from other projects'
websites.

## Core metrics

| Metric | cljgo | Reproduce |
|---|---|---|
| Tool binary | 12.7 MB stripped | `go build -trimpath -ldflags="-s -w" ./cmd/cljgo` |
| Compiled binary, hello | 5.3 MB | `cljgo build hello.clj` (strips by default) |
| Compiled startup, hello | **5.0 ms** (was 28.9 ms pre-AOT-core) | `hyperfine -N ./hello` |
| Peak RSS, hello | 14.7 MB | `/usr/bin/time -l ./hello` |
| Interpreter boot | 31.7 ms · 44.5 MB · 733k allocs | `go test -bench BenchmarkBoot -benchmem ./pkg/eval/` |
| Emitted vs handwritten Go | ~5× (target ~10×, reached and passed 2026-07-23) | `go test -run TestFactorialPerfBudget ./pkg/emit/` |
| clojure-test-suite | 238 / 242 (98.3%) | `cljgo suite` |

A compiled binary starts in ~5 ms because it links the **compiled** core, never
the interpreter (ADR 0046) — the interpreter's ~32 ms boot is the REPL/dev
path only.

## Head-to-head: let-go's own suite, unmodified

[let-go](https://github.com/nooga/let-go) is the closest comparable — Clojure
on Go; this table runs its bytecode VM (`lg file.clj`), its AOT leg is in the
head-to-head below. cljgo ran **let-go's own
benchmark suite** with let-go's published methodology (hyperfine, 3 warmup /
10 runs). All 7 files run on cljgo with no edits. Wall-clock mean of 10 runs,
**startup included** — the honest mode. Best per row in bold.

The field is mixed execution modes, so both cljgo legs are shown and compared
like-for-like: **interpreted** — `cljgo run` and joker are tree-walkers;
**compiled** — `cljgo` (native AOT binary, what you ship), babashka (GraalVM
native image), let-go (bytecode VM), Clojure JVM (JIT).

| Benchmark | cljgo run (interp) | cljgo (AOT) | let-go | babashka | joker (interp) | clojure JVM |
|---|---|---|---|---|---|---|
| startup | 38.5 ms | **5.0 ms** | **5.0 ms** | 9.7 ms | 6.7 ms | 289.2 ms |
| `tak` | 11.47 s | **34.6 ms** | 1.33 s | 1.14 s | — | 457.9 ms |
| `fib` | 8.79 s | **24.7 ms** | 1.25 s | 1.14 s | — | 419.1 ms |
| `loop-recur` | 469.3 ms | **5.4 ms** | 36.6 ms | 37.8 ms | 437.8 ms | 397.5 ms |
| `persistent-map` | 48.5 ms | **9.4 ms** | 12.9 ms | 13.0 ms | 30.5 ms | 385.2 ms |
| `map-filter` | 39.9 ms | 5.1 ms | **4.8 ms** | 10.0 ms | 8.4 ms | 311.5 ms |
| `transducers` | 89.0 ms | 16.4 ms | 25.4 ms | **13.0 ms** | — | 315.9 ms |
| `reduce` | 61.4 ms | 26.0 ms | 22.8 ms | **20.0 ms** | 1.46 s | 302.5 ms |
| runtime size | — | **12.7 MB** | 13 MB | 67.9 MB | 27.4 MB | 385.0 MB |

Versions: cljgo @HEAD (post-ADR-0067) · let-go v1.11.1 (tag, built from source
with the same toolchain and flags) · babashka v1.12.218 · joker v1.9.0 ·
Clojure CLI 1.12.5.1645 on OpenJDK 26.0.1. joker has no `transducers` and is
skipped on `fib`/`tak` (~13× slower there). Runtime size is the stripped
binary for cljgo/let-go, the installed binary for babashka/joker, and
JDK + `clojure.jar` (381.0 + 4.0 MB) for the JVM; a *compiled cljgo program*
is 5.3 MB. Not measured (and honestly flagged as such): **go-joker** (needs a
source clone + codegen). Reproduce the whole table: `bash benchmark/run.sh` —
methodology in
[`benchmark/README.md`](https://github.com/muthuishere/cljgo/blob/main/benchmark/README.md).

### Two honest reads of that table

**The good.** The AOT leg wins every recursion and data-structure row
outright: `tak` 34.6 ms and `fib` 24.7 ms (13× and 17× faster than the JVM —
pure int64 recursion is exactly what the ADR 0067 numeric-inference pass lifts
to raw typed Go: `func fib(n int64) int64`, direct recursion, zero boxing),
`loop-recur` 5.4 ms, `persistent-map` 9.4 ms. `startup` is a dead heat with
let-go at 5.0 ms, and `map-filter` is 0.3 ms behind — a tie.

**The bad.** `transducers` (16.4 vs babashka's 13.0 ms) and `reduce` (26.0 vs
babashka's 20.0, let-go's 22.8) remain behind the two purpose-built cores —
closer than before, but honestly lost. The residual is per-element `Apply2`
dispatch on the reducing fn plus `LongChunk.Nth` boxing; ADR 0067's follow-ups
and an unboxed internal-reduce are the named path. And the interpreter leg
(`cljgo run`) is a tree-walker: it loses everywhere except against joker.
That is what `cljgo build` is for.

## AOT vs AOT vs AOT: the compiled-Clojure-on-Go head-to-head

Three projects compile Clojure to Go source and then to a native binary:
cljgo, [Glojure](https://github.com/glojurelang/glojure), and
[let-go](https://github.com/nooga/let-go). This is the like-for-like
comparison — **every column is a native binary built from the same program**,
no interpreted legs. Glojure and let-go binaries were built with
[gloat](https://github.com/gloathub/gloat) (`-E glj` and `-E lglvm`), the
official automation tool for both. Measured **2026-07-24**, hyperfine
3 warmup / 10 runs, wall-clock mean, startup included, compile time excluded.
Best per row in bold.

| Benchmark | cljgo (AOT) | Glojure (AOT) | let-go (AOT) |
|---|---|---|---|
| startup | 4.7 ms | **3.6 ms** | 5.1 ms |
| `tak` | **36.4 ms** | 50.6 ms | 59.6 ms |
| `fib` | **24.1 ms** | 37.4 ms | 65.8 ms |
| `loop-recur` | 5.9 ms | **3.7 ms** | 37.3 ms |
| `persistent-map` | 10.5 ms | **7.4 ms** | 12.6 ms |
| `map-filter` | 6.3 ms | **3.8 ms** | 5.3 ms |
| `transducers` | 17.0 ms | **9.9 ms** | 25.4 ms |
| `reduce` | 26.8 ms | **23.2 ms** | 39.4 ms |
| binary size | **5.3–6.7 MB** | 7.5 MB | 12.8 MB |

**Honest read: Glojure wins this table** — 6 of 8 rows. Its codegen does
int64/float64 specialization, direct-call targets, and reduce-pipeline fusion,
and it shows. cljgo takes the tree-recursion rows (`tak`, `fib` — the ADR 0067
numeric-inference pass) and the smallest binaries; let-go's lowered leg trails
because values stay VM-boxed. Where the three still differ architecturally:
cljgo compiles source forms without evaluating them and links **zero
interpreter** into the binary (CI-checked); Glojure's generator walks an
evaluated namespace and its AOT builds retain the evaluator; let-go's lowered
binaries keep the VM runtime linked.

Versions: cljgo @HEAD · gloat v0.1.62 pinning Glojure v0.7.0 and let-go
v1.12.2 (gloat builds with its own pinned Go toolchain; cljgo with the repo
toolchain). let-go's `transducers` used gloat's pure-retry fallback (its
LG-overrides pass failed to build). gloat's pure `lgl` engine (no VM) is not
implemented yet; `lglvm` is its shipping AOT mode. Reproduce:
`bash benchmark/run-aot.sh` after building the three binary sets — steps in
[`benchmark/README.md`](https://github.com/muthuishere/cljgo/blob/main/benchmark/README.md).

## Web framework (bri) vs the field

[bri](/cljgo/bri/http/) (cljgo's web framework) AOT-compiles to a single static
`CGO_ENABLED=0` binary and deploys as a minimal Docker image, byte-identical to
the interpreter path (ADR 0071). Measured **2026-07-24** on Apple M-series
arm64, Docker/OrbStack, [`oha`](https://github.com/hatoo/oha) 15 s @ 50
connections, **one container at a time** (contention skews numbers). Every
server answers the same two routes with byte-exact bodies (`GET /` → `hello\n`;
`GET /api/hello` → `{"msg":"hello from <runtime>"}`). Reproduce with
`spikes/s45-bri-aot-docker/bench/run.sh` (corpus + runner committed).

| runtime | image | cold-start | `/` req/s | `/api` req/s | p99 | peak RSS |
|---|--:|--:|--:|--:|--:|--:|
| rust-axum | 140 MB | 28 ms | 89,480 | 89,986 | ~1.0 ms | 8 MB |
| deno | 277 MB | 146 ms | 89,316 | 89,099 | ~0.9 ms | 21 MB |
| clj-httpkit (JVM) | 847 MB | 1277 ms | 82,837 | 83,669 | ~1.0 ms | 353 MB |
| **bri (cljgo, compiled)** | **15.5 MB** | **~30 ms** | **78,126** | **77,788** | **1.4 ms** | **~16 MB** |
| bun | 333 MB | 28 ms | 74,798 | 83,535 | ~1.5 ms | 50 MB |
| clj-ring-jetty (JVM) | 858 MB | 1659 ms | 67,786 | 67,442 | ~1.5 ms | 491 MB |
| dotnet (ASP.NET) | 359 MB | 172 ms | 62,792 | 67,451 | ~1.9 ms | 47 MB |
| go net/http | 7.6 MB | 30 ms | 66,876 | 55,769 | ~2.6 ms | 16 MB |
| node | 228 MB | 147 ms | 55,344 | 62,167 | ~1.8 ms | 134 MB |
| spring-boot (JVM) | 512 MB | 858 ms | 51,002 | 55,056 | ~1.7 ms | 574 MB |
| fastapi (python) | 220 MB | 381 ms | 8,931 | 8,948 | ~10.5 ms | 38 MB |

**bri vs JVM Clojure** (the design bet): ~55× smaller image, ~40–55× faster
cold-start, ~22–30× less memory, comparable-or-better throughput. bri also
out-throughputs Go net/http, Node, .NET, Spring Boot, and FastAPI, sitting in
the top tier with Rust/Deno/Bun/http-kit. Throughput has run-to-run noise on a
single arm64 laptop (Go's `/` ranged 66–70k across runs); the image / RAM /
cold-start figures are stable. The point is not a leaderboard crown — it is
that a Clojure web app can ship as a ~15 MB, ~30 ms-start, ~16 MB-RAM native
binary. See [Deploy](/cljgo/guides/deploy/) for the Dockerfile.

## Footprint & density

Throughput is a fair fight; memory and size are not. Same program
(`(reduce + (range 1000))`), max resident set via `/usr/bin/time -l`:

| Runtime | Static binary / install | Max RSS |
|---|---|---|
| cljgo | **5.3 MB** static binary | **14 MB** |
| joker | 27.4 MB | 27 MB |
| babashka | 67.9 MB | 28 MB |
| JVM Clojure | JVM + classpath | 102 MB |

That is ~7× less memory than JVM Clojure, measured — and it's the JVM's
*best* case: a hello that exits. On the same program cljgo also finished in
**9 ms total wall-clock** vs JVM Clojure's **298 ms**, boot included. (A
CI-gated peak-RSS budget is on the roadmap so "low memory" becomes an enforced
promise, not a slide.)

## The 2026-07-23 campaign (ADRs 0063–0067)

Five decisions moved emitted code from "correct but boxed" to competitive:
chunk-aware `map`/`filter`/`count`/`keep`, the IFn2 2-arg reduce seam,
direct-call emission for known local fns, the sealed-core dirty flag, int64
numeric inference, plus `<=`/`>=` unboxed compares and a startup clawback.
Same machine, morning → evening:

| Benchmark | before | after | speedup |
|---|---|---|---|
| `fib` | 975.4 ms | **24.7 ms** | 39× |
| `tak` | 858.5 ms | **34.6 ms** | 25× |
| `loop-recur` | 52.1 ms | **5.4 ms** | 9.6× |
| `reduce` | 60.8 ms | **26.0 ms** | 2.3× |
| startup | 6.5 ms | **5.0 ms** | after a mid-day 9.5 ms peak |

The emitted-vs-handwritten-Go factorial gate measures **~5×** — it was ~35×
before this campaign and ~168× under naive emission, and the ~10× target of
design/00 §1.4 is passed for the first time.

## Budgets are gates, not vibes

Two budgets run inside plain `go test ./...`, host-relative because a CI
runner is not your laptop (override with `CLJGO_BOOT_BUDGET` /
`CLJGO_PERF_RATIO_MAX`):

- **Interpreter boot** — `TestBootUnderBudget`, 250 ms locally
  (`pkg/eval/boot_test.go`). Boot got 8.9× faster in v0.2.0 (211 ms →
  23.7 ms, by killing a stack-scraping goroutine-ID lookup that burned 73% of
  boot CPU); it is 31.7 ms today, grown with the core it loads.
- **Emitted vs handwritten Go** — `TestFactorialPerfBudget`, 15× ceiling
  (`pkg/emit/perf_test.go`). The 15× gate is a regression guard, not the
  target; measured ~5×.

Compiled-binary startup is also CI-gated (`TestBootStartupBudget`) since the
2026-07-23 clawback, so the 5.0 ms cannot silently drift again.
