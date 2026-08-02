---
title: Benchmarks
description: Measured, reproducible numbers — boot time, memory, binary size, and a head-to-head against let-go, babashka, joker, and JVM Clojure. Wins and losses both published.
---

Performance in cljgo is a gated feature, not a marketing claim: perf budgets
run inside plain `go test ./...` and a regression is treated like a
conformance failure. Every number on this page was **measured, not quoted** —
and the rows cljgo loses are published alongside the ones it wins.

**Measurement context:** Apple M5 Pro, darwin/arm64, go1.26.3. `hello.clj` =
`(println "hi")`. Every competing runtime was installed and measured on the
same machine — no normalization, no numbers copied from other projects'
websites. **Each table carries its own date and cljgo version**, because they
were not all measured at the same time.

:::caution[Which tables are current, and which are not]
Everything on this page was re-measured on **2026-08-02** against cljgo
**post-v0.8.9** (commit `f46b9a8` — v0.8.9 plus eight commits), *except* the
one table explicitly marked historical:

- **Current, 2026-08-02:** [core metrics](#core-metrics),
  [the interpreted-runtime comparison](#head-to-head-let-gos-own-suite-unmodified),
  [AOT vs AOT vs AOT](#aot-vs-aot-vs-aot-the-compiled-clojure-on-go-head-to-head),
  [the web table](#web-framework-bri-vs-the-field), and
  [footprint & density](#footprint--density).
- **Historical, kept as a record:** [the 2026-07-23 campaign
  table](#the-2026-07-23-campaign-adrs-00630067). It is a before/after of one
  day's work and is dated as such; do not read its "after" column as a
  current number.

Two things in the AOT head-to-head changed shape this round and are called out
where they appear. First, the Glojure and let-go binaries were **rebuilt**,
not re-timed — the previous artifacts no longer existed — and the rebuild does
not reproduce the older Glojure figures on the same pinned version. Second,
run-to-run σ was 0.3–1.0 ms, which makes four of the eight rows ties rather
than wins.
:::

## Core metrics

Measured **2026-08-02**, cljgo **post-v0.8.9** (`f46b9a8`).

| Metric | cljgo | Reproduce |
|---|---|---|
| Tool binary | 28.6 MB stripped | `go build -trimpath -ldflags="-s -w" ./cmd/cljgo` |
| Compiled binary, hello | 7.1 MB (7,083,298 B) | `cljgo build hello.clj` (strips by default) |
| Compiled startup, hello | **5.1 ms** ± 0.5 (was 28.9 ms pre-AOT-core) | `hyperfine -N ./hello` |
| Peak RSS, hello | 12.4 MB | `/usr/bin/time -l ./hello` |
| Interpreter boot | 38.8 ms · 54.2 MB · 875k allocs | `go test -bench BenchmarkBoot -benchmem ./pkg/eval/` |
| Emitted vs handwritten Go | ~3.8× (target ~10×, reached and passed 2026-07-23) | `go test -run TestFactorialPerfBudget ./pkg/emit/` |
| clojure-test-suite | 238 / 242 (98.3%) | `cljgo suite` |

A compiled binary starts in ~5 ms because it links the **compiled** core, never
the evaluator (ADR 0046) — the interpreter's boot is the REPL/dev path only.

Two of these moved the wrong way since the v0.8.2 measurement and are not
being buried. **Interpreter boot grew from 31.7 ms / 44.5 MB / 733k allocs to
38.8 ms / 54.2 MB / 875k allocs** — a 22% slower REPL start, tracking the core
that boot loads; it is still far inside the 250 ms `TestBootUnderBudget` gate,
which means the gate is not tight enough to have caught it. **The compiled
binary grew from 6.7 MB to 7.1 MB.** Neither is a trade-off anyone chose.

One caveat retired: hello-world used to be quoted as a *smaller* program than
the benchmark-suite binaries. It is not any more — `(println "hi")` compiles
to 7,083,298 bytes, byte-identical to six of the seven suite binaries, because
the linked AOT core dominates and the program itself is noise.

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
| startup | 47.0 ms | **5.50 ms** | 5.51 ms | 10.9 ms | 7.0 ms | 302.0 ms |
| `tak` | 11.17 s | **37.6 ms** | 1.34 s | 1.13 s | — | 435.7 ms |
| `fib` | 8.53 s | **24.6 ms** | 1.25 s | 1.14 s | — | 427.6 ms |
| `loop-recur` | 461.5 ms | **6.0 ms** | 36.8 ms | 38.3 ms | 432.5 ms | 385.4 ms |
| `persistent-map` | 56.6 ms | **11.1 ms** | 12.8 ms | 12.5 ms | 30.3 ms | 394.1 ms |
| `map-filter` | 48.7 ms | 5.8 ms | **4.9 ms** | 10.5 ms | 8.6 ms | 314.2 ms |
| `transducers` | 96.1 ms | 17.8 ms | 25.3 ms | **13.5 ms** | — | 321.2 ms |
| `reduce` | 71.0 ms | 28.6 ms | 23.9 ms | **21.8 ms** | 1.47 s | 304.0 ms |
| runtime size | — | 28.6 MB | **13.2 MB** | 71.2 MB | 28.8 MB | 403.2 MB |

Measured **2026-08-02**. Versions: cljgo **post-v0.8.9** (`f46b9a8`) ·
let-go `main` @ `0e56abd` (2026-07-14, untagged — after the v1.11.1 tag, and
it includes the loop-fusion pass; built from source with the same toolchain
and flags) · babashka v1.12.218 · joker v1.9.0 · Clojure CLI 1.12.5.1645 on
OpenJDK 26.0.1. joker has no `transducers` and is skipped on `fib`/`tak`
(~13× slower there). Runtime size is the stripped binary for cljgo/let-go,
the installed binary for babashka/joker, and JDK + `clojure.jar`
(399.0 + 4.2 MB) for the JVM; a *compiled cljgo program* is 7.1 MB. Not
measured (and honestly flagged as such): **go-joker** (needs a source clone +
codegen), and Glojure, whose AOT leg is in the head-to-head below.
Reproduce: `bash benchmark/run.sh` — methodology in
[`benchmark/README.md`](https://github.com/muthuishere/cljgo/blob/main/benchmark/README.md).

### Two honest reads of that table

**The good.** The AOT leg wins every recursion and data-structure row
outright: `tak` 37.6 ms and `fib` 24.6 ms (12× and 17× faster than the JVM —
pure int64 recursion is exactly what the ADR 0067 numeric-inference pass lifts
to raw typed Go: `func fib(n int64) int64`, direct recursion, zero boxing),
`loop-recur` 6.0 ms, `persistent-map` 11.1 ms. `startup` is a genuine dead
heat with let-go — 5.50 vs 5.51 ms, σ ~0.7, so the bold is arithmetic, not a
win — and `map-filter` is 0.9 ms behind.

**The bad.** `transducers` (17.8 vs babashka's 13.5 ms) and `reduce` (28.6 vs
babashka's 21.8, let-go's 23.9) remain behind the two purpose-built cores, and
both drifted slightly further away since the pre-v0.7.0 run. The residual is
per-element `Apply2` dispatch on the reducing fn plus `LongChunk.Nth` boxing;
ADR 0067's follow-ups and an unboxed internal-reduce are the named path. The
interpreter leg (`cljgo run`) is a tree-walker: it loses everywhere except
against joker, and its **startup regressed from 38.5 ms to 47.0 ms** — same
growth in the booted core that the interpreter-boot benchmark above shows.
That is what `cljgo build` is for, but it is a REPL cost users feel.

## AOT vs AOT vs AOT: the compiled-Clojure-on-Go head-to-head

Three projects compile Clojure to Go source and then to a native binary:
cljgo, [Glojure](https://github.com/glojurelang/glojure), and
[let-go](https://github.com/nooga/let-go). This is the like-for-like
comparison — **every column is a native binary built from the same program**,
no interpreted legs. The programs are
[let-go's own benchmark suite](https://github.com/nooga/let-go/tree/main/benchmark)
(vendored unmodified — credit nooga). Glojure and let-go binaries were built with
[gloat](https://github.com/gloathub/gloat) (`-E glj` and `-E lglvm`), the
official automation tool for both. Re-measured **2026-08-02** on cljgo
**post-v0.8.9** (`f46b9a8`), hyperfine
3 warmup / 10 runs, wall-clock mean, startup included, compile time excluded.
**All three binary sets were rebuilt for this run.** Bold marks the
arithmetic minimum — read it with the σ column, because four rows are ties.

| Benchmark | cljgo (AOT) | Glojure (AOT) | let-go (AOT) | σ (worst leg) | verdict |
|---|---|---|---|---|---|
| startup | 6.1 ms | 6.5 ms | **6.0 ms** | 1.0 ms | tie |
| `tak` | **36.6 ms** | 53.1 ms | 58.8 ms | 2.5 ms | cljgo, 1.45× |
| `fib` | **25.6 ms** | 39.5 ms | 65.4 ms | 1.2 ms | cljgo, 1.54× |
| `loop-recur` | **6.5 ms** | 6.7 ms | 36.6 ms | 0.9 ms | tie (vs Glojure) |
| `persistent-map` | **10.8 ms** | 10.9 ms | 12.6 ms | 1.0 ms | tie |
| `map-filter` | 6.4 ms | 6.3 ms | **5.8 ms** | 0.9 ms | tie |
| `transducers` | 17.2 ms | **11.7 ms** | 24.7 ms | 0.6 ms | Glojure, 1.47× |
| `reduce` | 27.2 ms | **7.4 ms** | 40.6 ms | 1.0 ms | **Glojure, 3.67×** |
| binary size | **7.1 MB** | 19.0 MB | 12.8 MB | — | cljgo |

**Honest read: it is two clear wins each, four ties, and one bad loss.** The
old "Glojure wins 6 of 8" summary does not survive a run that reports its own
noise — `startup`, `loop-recur`, `persistent-map` and `map-filter` all sit
inside σ between cljgo and Glojure and should be read as level. cljgo takes
the tree-recursion rows (`tak`, `fib` — the ADR 0067 numeric-inference pass
lifts them to raw typed Go). Glojure takes `transducers` and, decisively,
**`reduce`: 7.4 ms against 27.2**.

That `reduce` row is the finding on this page, so it was checked rather than
reported. None of the seven programs prints anything, which means nothing in
the harness would notice a compiler eliding the work — so a printing variant
was compiled on both at N = 1M, 4M and 16M. Both print the correct sum
(499999500000 / 7999998000000 / 127999992000000) and both scale linearly:
Glojure 7.1 → 10.6 → 21.1 ms, cljgo 27.0 → 93.8 → 348.7 ms. Per element that
is **~1.0 ns for Glojure against ~21.8 ns for cljgo** — a genuine ~21×
per-element gap in the reduce path, not dead-code elimination, and a roadmap
gap rather than a trade-off anyone chose.

Two claims from the previous run **did not reproduce and have been dropped**,
not silently updated:

- **"cljgo starts slower than Glojure."** At v0.8.2 that read 5.1 vs 3.9 ms.
  Against freshly built binaries it reads 6.1 vs 6.5 ms with σ around 1.0, and
  a confirmation re-run read 5.0 / 5.4 / 4.6 with the same ordering. Startup
  is a three-way tie here. That is not a win either — it is an absence of a
  measurable difference.
- **"Glojure's binary is 7.5 MB."** Rebuilt from the same gloat v0.1.62
  pinning the same glj v0.7.0, with the same `-tags glj_aot_runtime -ldflags
  "-s -w"`, it measures **19.0 MB** (19,021,826 bytes). The 2026-07-24
  artifacts that produced the 7.5 MB figure no longer exist, so the
  discrepancy cannot be fully diagnosed — but it can be narrowed, and the
  narrowing matters:

  **let-go, rebuilt in the same run with the same tooling, reproduces its
  previously recorded figure to the byte-ish: 12,838,082 B against 12.8 MB.**
  A control that lands on the old number is strong evidence the current
  measurement setup is sound, and therefore that the 7.5 MB Glojure figure —
  not this 19.0 MB one — is the outlier. The likeliest explanation is that
  the older Glojure artifact was built without `-tags glj_aot_runtime`, the
  tag that retains the evaluator and reader; that would explain both a much
  smaller binary and why it was ever believed. It is a hypothesis, not a
  measurement, and it is labelled as one.

  What follows for public claims: quote 19.0 MB because it is what was
  measured, say the earlier figure is unreproducible, and do **not** claim
  "Glojure's binary grew 2.5×" — nothing here measured a change over time.
  cljgo's own suite binaries are 7,083,298 bytes (`tak` 7,099,810), up from
  7,049,666 at v0.8.2.

Where the three differ architecturally — and this is where the previous
wording overclaimed. A cljgo AOT binary links **no evaluator and no
analyzer**: `strings aot_fib | grep -c cljgo/pkg/eval` is 0, as are
`pkg/analyzer`, `pkg/ast` and `pkg/repl`, and CI enforces it
(`pkg/coreaot/imports_test.go`). It does link the **reader** — 121 symbols
under `cljgo/pkg/reader`, because `read` and `read-string` are ordinary
runtime core functions — plus `pkg/emit/rt`, the bootstrap. So the accurate
claim is *evaluator-free*, not *interpreter-free*; the page previously said
the probes "return nothing on a cljgo binary", which is only true of the
Glojure-specific probe strings. On the Glojure side both are present:
`grep -c EvalAST` → 57 and `grep -c glojure/pkg/reader` → 61 on `fib-glj`
(stripped binaries keep Go's pclntab, so names survive `-s -w`). let-go's
lowered binaries keep the VM (`let-go/pkg/vm` → 1939). We don't claim any of
this accounts for the whole size delta — only that Glojure ships an evaluator
and cljgo does not.

Versions: cljgo post-v0.8.9 `f46b9a8`, rebuilt 2026-08-02 with the repo Go
toolchain (go1.26.3) · gloat v0.1.62 pinning Glojure v0.7.0 and let-go
v1.12.2, building with its own pinned toolchain. Unlike the 2026-07-31 run,
the Glojure and let-go binaries here were **rebuilt from source**, because
the earlier artifacts were gone; timings are therefore comparable within this
table and the competitor versions are the gloat-pinned ones. let-go's
`transducers` again used gloat's pure-retry fallback (its LG-overrides pass
failed to build). gloat's pure `lgl` engine (no VM) is not implemented yet;
`lglvm` is its shipping AOT mode. Reproduce:
`bash benchmark/run-aot.sh` after building the three binary sets — steps in
[`benchmark/README.md`](https://github.com/muthuishere/cljgo/blob/main/benchmark/README.md).

## Web framework (bri) vs the field

[bri](/cljgo/bri/http/) (cljgo's web framework) AOT-compiles to a single static
`CGO_ENABLED=0` binary and deploys as a minimal Docker image, byte-identical to
the interpreter path (ADR 0071).

Measured **2026-08-02**, cljgo **post-v0.8.9** (`f46b9a8`), darwin/arm64,
OrbStack 29.4.0, [`oha`](https://github.com/hatoo/oha) 20 s @ 50 connections
after a 3 s warm-up, **one benchmark container at a time**. Every server
answers the same two routes with byte-exact bodies (`GET /` → `hello\n`;
`GET /api/hello` → `{"msg":"hello from <runtime>"}`). The whole eleven-runtime
field was re-run in this one session, so unlike the previous two rounds there
is no superseded row and no patched cell. Sorted by `/api` throughput.

| runtime | image | cold-start | `/` req/s | `/api` req/s | `/api` p99 | peak RSS |
|---|--:|--:|--:|--:|--:|--:|
| rust-axum | 140 MB | 27 ms | 78,957 | **79,878** | 0.92 ms | 8.6 MB |
| bun | 333 MB | 138 ms | 78,152 | 76,165 | 1.63 ms | 16.6 MB |
| clj-httpkit (JVM) | 847 MB | 1364 ms | 74,170 | 75,941 | 0.99 ms | 321.7 MB |
| deno | 277 MB | 258 ms | 80,101 | 75,230 | 1.39 ms | 41.7 MB |
| spring-boot (JVM) | 512 MB | 958 ms | 69,814 | 69,722 | 1.28 ms | 660.5 MB |
| go net/http | **7.6 MB** | **26 ms** | 66,922 | 66,425 | 2.33 ms | 14.7 MB |
| dotnet (ASP.NET) | 359 MB | 169 ms | 64,831 | 66,015 | 1.62 ms | 46.7 MB |
| node | 228 MB | 59 ms | 61,973 | 61,782 | 1.57 ms | 71.1 MB |
| clj-ring-jetty (JVM) | 858 MB | 1656 ms | 56,500 | 56,757 | 1.63 ms | 440.2 MB |
| **bri (cljgo, compiled)** | 20.1 MB | 29 ms | 39,263 | 38,206 | 4.56 ms | 34.7 MB |
| fastapi (python) | 220 MB | 480 ms | 9,529 | 9,616 | 8.34 ms | 41.0 MB |

The bri entrant is the flagship `bri.http` app in `benchmark/web/bri` running
`http/listen` — the **zero-config default**, which applies seven middleware
layers (request-id, logging, recover, cors, metrics, auto-ban, negotiate). No
other entrant in the corpus does per-request logging, metrics or abuse
protection at all, so this is not a like-for-like handler comparison: bri's
default is doing more work than every other row, and paying for it. At
v0.8.2, stripping back to `serve` with recover only measured 58,222 req/s
against `listen`'s 31,404 — a 46% swing that has not been re-measured here.

**The honest read: bri is tenth of eleven on throughput, and second on
footprint.** Two things are true at once and neither cancels the other.

What holds, and is structural rather than a tuning result: at **20.1 MB** the
image is ~42× smaller than clj-httpkit's 847 MB, cold-start is **29 ms**
against 1364 ms (~47×), and peak RSS is **34.7 MB** against 321.7 MB (~9×).
Only the bare Go net/http server ships smaller or starts faster, and that is a
handler, not a framework. A Clojure web app really does ship as a ~20 MB,
~30 ms-start native binary, and that was always the design bet.

What does not hold, and is a roadmap gap rather than a trade-off anyone chose:
throughput. bri serves **38,206 req/s** against clj-httpkit's 75,941 — it is
**~2× below JVM Clojure**, below every other entrant except FastAPI, and its
p99 (4.56 ms) is the second-worst in the field. The claims "comparable-or-better
throughput" and "top tier with Rust/Deno/Bun/http-kit" that appeared here
before 2026-07-31 rested on a row that never reproduced; they are retracted and
not coming back until a measurement earns them.

Attribution is in spikes `s76` (per-layer, in-process) and `s72`. Optimising
the request path 2.3× moved the end-to-end number by 0–2% — at 58k req/s a
request has ~17 µs of budget and the request map was ~0.9 µs of it, so that
was never where the time went. What is still unexplained is the gap between
bri's bare stack and http-kit on the compiled path.

Two exclusions worth naming. This is a single arm64 laptop, so throughput
carries real run-to-run noise (Go's `/` has ranged 66–70k across sessions);
cells are comparable **within** this table only. And the host had several
idle, unrelated containers (Postgres, Redis, MinIO) resident throughout —
they were quiescent, but "one container at a time" describes the *benchmark*
containers, not the machine.

Reproduce with
[`bash benchmark/web/run.sh`](https://github.com/muthuishere/cljgo/blob/main/benchmark/web/README.md)
— corpus, Dockerfiles and runner are all committed, and the bri image builds
cljgo from your checkout rather than a release. See
[Deploy](/cljgo/guides/deploy/) for the Dockerfile.

## Footprint & density

Throughput is a fair fight; memory and size are not. Same program
(`(reduce + (range 1000))`), max resident set via `/usr/bin/time -l`,
measured **2026-08-02**:

| Runtime | Static binary / install | Max RSS |
|---|---|---|
| cljgo | **7.1 MB** static binary | **13.9 MB** |
| joker | 28.8 MB | 27.8 MB |
| babashka | 71.2 MB | 30.2 MB |
| JVM Clojure | 403.2 MB (JDK 399.0 + `clojure.jar` 4.2) | 101.6 MB |

That is ~7× less memory than JVM Clojure, measured — and it's the JVM's
*best* case: a hello that exits. On the same program cljgo also finished in
**5.5 ms total wall-clock** vs JVM Clojure's **303.5 ms**, boot included. (A
CI-gated peak-RSS budget is on the roadmap so "low memory" becomes an enforced
promise, not a slide.)

## The 2026-07-23 campaign (ADRs 0063–0067)

*Historical. This is a before/after of one day's work at the end of July 2026;
its "after" column is a 2026-07-23 measurement, not a current number. Current
figures are in the tables above.*

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

The emitted-vs-handwritten-Go factorial gate measured **~5×** at the end of
that campaign — it was ~35× before it and ~168× under naive emission, and the
~10× target of design/00 §1.4 was passed for the first time. It measures
**~3.8×** today (2026-08-02).

## Budgets are gates, not vibes

Two budgets run inside plain `go test ./...`, host-relative because a CI
runner is not your laptop (override with `CLJGO_BOOT_BUDGET` /
`CLJGO_PERF_RATIO_MAX`):

- **Interpreter boot** — `TestBootUnderBudget`, 250 ms locally
  (`pkg/eval/boot_test.go`). Boot got 8.9× faster in v0.2.0 (211 ms →
  23.7 ms, by killing a stack-scraping goroutine-ID lookup that burned 73% of
  boot CPU); it is **38.8 ms** today (2026-08-02), grown with the core it
  loads — up from 31.7 ms at v0.8.2. A 250 ms ceiling did not notice that,
  which is a fair criticism of the gate rather than of the number.
- **Emitted vs handwritten Go** — `TestFactorialPerfBudget`, 15× ceiling
  (`pkg/emit/perf_test.go`). The 15× gate is a regression guard, not the
  target; measured **~3.8×** (2026-08-02).

Compiled-binary startup is also CI-gated (`TestBootStartupBudget`) since the
2026-07-23 clawback, so the ~5 ms cannot silently drift again.
