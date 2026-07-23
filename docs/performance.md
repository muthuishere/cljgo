# Performance

Performance is priority 4 and gated like conformance, not asserted. Every
number here was measured on the same machine (Apple M5 Pro, go1.26.3, cljgo
@HEAD) — no normalization, no quoted numbers, wall-clock mean of 10 runs.
Reproduce the head-to-head with **`bash benchmark/run.sh`** (the committed
corpus + runner + report); see [`benchmark/README.md`](../benchmark/README.md)
for methodology, binary sizes, and the `reduce` gap analysis.

## Measured facts

With `hello.clj` = `(println "hi")`:

| | cljgo | reproduce |
|---|---|---|
| Tool binary | 12.7 MB stripped | `go build -trimpath -ldflags="-s -w" ./cmd/cljgo` |
| Compiled binary, hello | 5.3 MB | `cljgo build hello.clj` (strips by default) |
| Compiled startup, hello | 5.0 ms | `hyperfine -N ./hello` |
| Peak RSS, hello | 14.7 MB | `/usr/bin/time -l ./hello` |
| Interpreter boot | 31.7 ms · 44.5 MB · 733k allocs | `go test -bench BenchmarkBoot -benchmem ./pkg/eval/` |
| clojure-test-suite | 238/242 (98.3%) | `cljgo suite` |

**Building an AOT binary is one command** — `cljgo build -o hello hello.clj`.
It emits Go and invokes `go build`, so it needs the Go toolchain on `PATH`
(`cljgo run` / `cljgo repl` do not); it strips by default; and it links the
**compiled** core, never the interpreter (ADR 0046), which is why a compiled
binary starts in ~10 ms instead of the interpreter's ~32 ms boot.

## The two CI budgets

Both run inside plain `go test ./...`, and are host-relative because a CI
runner is not your laptop (ADR 0024) — override with `CLJGO_BOOT_BUDGET` and
`CLJGO_PERF_RATIO_MAX`:

- **Interpreter boot** — `TestBootUnderBudget`, 250 ms locally (`pkg/eval/boot_test.go`).
- **Emitted vs handwritten Go** — `TestFactorialPerfBudget`, 15× ceiling
  (`pkg/emit/perf_test.go`). Measured: **~4.8×** — under the ~10× target of
  design/00 §1.4 for the first time (ADR 0067; naive emission was ~168×,
  boxed emission ~35×). The 15× gate is a regression guard, not the target.

## The 2026-07-23 campaign — ADRs 0063–0067

Five decisions moved emitted code from "correct but boxed" to competitive:
chunk-aware `map`/`filter`/`count`/`keep` (JVM 32-element realization
parity, ADR 0063), the IFn2 2-arg seam (no `[]any` box per reduce step),
direct-call emission for statically-known local fns (0064), the sealed-core
dirty-flag (guard elision with full `with-redefs` liveness, 0066), and an
int64 numeric-inference pass that lifts monomorphic kernels to raw typed Go
(`func tak(x, y, z int64) int64`, 0067).

### Head-to-head vs let-go

[let-go](https://github.com/nooga/let-go) (v1.11.1) is the closest comparable —
Clojure on Go, but a bytecode VM rather than AOT-to-Go-source. Both built from
source on the same machine with the same toolchain and the same
`-trimpath -ldflags="-s -w"`, so this is not a spec-sheet comparison.

Run on **let-go's own benchmark suite**, unmodified, with let-go's published
methodology (hyperfine, 3 warmup / 10 runs). All 7 files compile and run on
cljgo with no edits. Totals include each runtime's startup. Best per row in
bold. `cljgo run` is the interpreter, `cljgo` is the AOT binary (`cljgo build`)
— what you ship.

| Benchmark | cljgo run | cljgo | let-go | babashka | joker | clojure JVM |
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

Versions: cljgo @HEAD (post-ADR-0067), let-go v1.11.1 (tag, built from source),
babashka v1.12.218, joker v1.9.0, Clojure CLI 1.12.5.1645 on OpenJDK 26.0.1.
`joker` has no `transducers` and is skipped on `fib`/`tak` (~13× slower there).
Runtime size is the stripped binary for cljgo / let-go, the installed binary for
babashka / joker, and JDK + `clojure.jar` (381.0 + 4.0 MB) for the JVM; a
*compiled cljgo program* is 5.3 MB. All seven benchmarks produce identical
values across all five runtimes (`persistent-map` differs only in hash-map print
order, so it is compared on `[count, sum-keys, sum-vals, (get m 9999)]`).
Not measured: **gloat** (its module exposes no installable package path) and
**go-joker** (needs a source clone + codegen) — let-go's published M1 Pro data
puts gloat at 12.7× let-go on `fib` and 5.4× on `reduce`.

### Two honest reads

**The good.** cljgo-aot now **wins every recursion and data-structure row
outright**: `tak` 34.6 ms and `fib` 24.7 ms (13× and 17× faster than the JVM —
pure int64 recursion is exactly what the ADR 0067 numeric-inference pass lifts
to raw Go: `func fib(n int64) int64`, direct typed recursion, zero boxing),
`loop-recur` 5.4 ms (6.8× ahead of let-go), and `persistent-map` 9.4 ms (ahead
of both let-go and babashka). `startup` is a dead heat with let-go at 5.0 ms —
the +3 ms regression the previous run reported was attributed (to boot-time
per-symbol refer and GC churn, *not* the seal/dual-body work, which measured
+0.0 ms) and clawed back with a bulk-refer snapshot + boot GC deferral, now
CI-gated so it cannot drift again. `map-filter` is 0.3 ms from let-go — a tie.
The emitted-vs-handwritten-Go factorial gate measures **~5×** — it was ~35×
before 2026-07-23.

**The bad.** `transducers` (16.4 vs babashka's 13.0 ms) and `reduce` (26.0 vs
babashka's 20.0, let-go's 22.8) remain behind the two purpose-built cores —
closer than the 2.4–2.8× of the previous run, but honestly lost. The residual
is per-element `Apply2` dispatch on the reducing fn plus `LongChunk.Nth`
boxing; ADR 0067's follow-ups (float64, multi-arity specialization, broader
lift) and an unboxed internal-reduce are the named path. The interpreter leg
(`cljgo run`) is a tree-walker and loses everywhere except against joker —
that is what `cljgo build` is for.

### Before / after (2026-07-23, two rounds)

Round 1: chunk-aware `map`/`filter`/`count`/`keep` (JVM 32-element realization
parity), the IFn2 2-arg seam (no `[]any` box per reduce step), direct-call
emission for known local fns, the sealed-core dirty-flag (guard elision with
full `with-redefs` liveness), and the int64 numeric-inference pass. Round 2:
`<=`/`>=` joined the unboxed compare set (fib's entire remaining cost), and
startup was attributed and clawed back. Same machine, morning → evening:

| Benchmark | before | after | |
|---|---|---|---|
| `fib` | 975.4 ms | **24.7 ms** | 39× |
| `tak` | 858.5 ms | **34.6 ms** | 25× |
| `loop-recur` | 52.1 ms | **5.4 ms** | 9.6× |
| `reduce` | 60.8 ms | **26.0 ms** | 2.3× |
| startup | 6.5 ms | **5.0 ms** | after a mid-day 9.5 ms peak |

## Earlier campaigns, kept honest

**AOT core (ADR 0046, spikes S22/S23).** Compiled `core.clj` and the boot
sources through cljgo's own emitter (`pkg/coreaot`): a compiled binary links
the **compiled** core and no interpreter at all (`pkg/eval` 155 → **0**
symbols in the link set, `pkg/analyzer` 63 → 0, `pkg/ast` 14 → 0 —
CI-enforced, `pkg/coreaot/imports_test.go`). At the time that took startup
27.5 → 5.5 ms; boot growth from the fundamentals batches later pushed it to
~9.5 ms, and the 2026-07-23 clawback (bulk boot refer + boot GC deferral,
now gated by `TestBootStartupBudget`) brought a hello binary to **~5.0 ms
today**. It also settled the A/B that used to indict `cljgo build`:
work in user code compiles to ~9.7× its interpreted speed, and core-heavy
programs stopped being indistinguishable between modes.

**What AOT core did not buy: size.** ADR 0023 §2 predicted ~2 MB per
compiled program. Measured then: 5.3 → 4.6 MB stripped — the tree-walker
left, but ~13k lines of compiled core arrived, and what remains is the
runtime a compiled binary genuinely needs (`pkg/lang`'s data structures and
numeric tower, `pkg/corelib`'s ~700 symbols, the reader). Dual-body emission
(ADR 0067) has since grown it back to **5.3 MB**. Size from here is a
dead-code and dual-body-trimming problem, not an AOT-core problem; ADR 0046
records the ~2 MB prediction as superseded by measurement.

**Boot.** Interpreter boot got 8.9× faster in v0.2.0 (211 ms → 23.7 ms) by
replacing a stack-scraping goroutine-ID lookup that was burning 73% of boot
CPU with a `getg()`-based one (ADR 0034, spike S18). It has since grown with
the core it boots — 31.7 ms today (ADR 0019 says the budget grows with the
core, and the 250 ms gate holds). `.github/workflows/boot-bench.yml` is a
manual ubuntu-vs-macos boot comparison kept as a permanent diagnostic.

## Web framework (bri) vs the field — ADR 0071 / spike s45

bri (cljgo's web framework) AOT-compiles to a single static `CGO_ENABLED=0`
binary and deploys as a minimal Docker image, byte-identical to the
interpreter path (dual-mode parity). Measured 2026-07-24 on Apple M-series
arm64, Docker/OrbStack, [`oha`](https://github.com/hatoo/oha) 15 s @ 50
connections, **one container at a time** (contention skews numbers). Every
server answers the same two routes with byte-exact bodies (`GET /` → `hello\n`
text/plain; `GET /api/hello` → `{"msg":"hello from <runtime>"}` JSON).
Reproduce with `spikes/s45-bri-aot-docker/bench/run.sh` (the corpus + runner
are committed).

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
binary. Full write-up: `spikes/s45-bri-aot-docker/VERDICT.md`.
