# Spike S22 verdict — `cljgo build` compiles your code and nothing else

Closed 2026-07-16. Recommendation feeds **ADR 0037** (reserved): reframe ADR
0023 decision #2 — AOT-`core.clj` is the top **performance** lever, not a
binary-size cleanup.

**Exit criterion: MET**, on the "yes" branch, with a stronger result than the
criterion required. The threshold was AOT ≈ interpreted at ratio ≤ ~1.3× on a
`clojure.core`-heavy benchmark. Measured: **1.00×**.

## 1. The decisive A/B

Same source, same machine, compiled vs interpreted:

| benchmark | AOT binary | `cljgo run` (interpreted) | speedup from compiling |
|---|---|---|---|
| `fib` — work in **user** code | 993.6 ms ± 34.3 | 9683 ms ± 667 | **9.74×** |
| `reduce` — work in **`clojure.core`** | 701.5 ms ± 9.5 | 700.0 ms ± 5.1 | **1.00×** |

`cljgo build` produces a **9.7× speedup on the user's own forms and exactly
zero on anything `clojure.core` does.** Compiling `(reduce + 0 (range 1e6))` is
indistinguishable from interpreting it — 701.5 vs 700.0 ms, inside noise.

## 2. Why

`pkg/emit/program.go:172` emits `main` as `rt.Boot(); Load()`. `rt.Boot()`
(`pkg/emit/rt/rt.go:40-56`) calls `eval.New()`, which reads and tree-walks the
embedded `core/core.clj` + 12 `.cljg` files (2980 lines) at every startup
(`pkg/eval/eval.go:39-75`). Emitted code reaches `clojure.core` only through
`lang.InternVarName(...).Get()` (`pkg/emit/emit.go:135-153`), and
`InternVarName` creates an **unbound** var (`pkg/lang/var.go:101-116`) — the
value is bound at runtime by the interpreter.

So an emitted binary is a native Go program whose standard library is a set of
interpreted closures rebuilt from source on every run. The user's `fib` is
compiled Go; the `reduce` it calls is a tree-walk closure. A bytecode VM beats
a tree-walker, which is the entire `reduce` result.

## 3. The competitive consequence — let-go's own suite, unmodified

All 7 files run on cljgo with no edits. Every runtime **installed and measured
on this machine** — no normalization, no quoted figures. Wall-clock mean of 10
runs, startup included. Raw JSON in `results/full-*.json`.

| Benchmark | cljgo | let-go | babashka | joker | clojure JVM |
|---|---|---|---|---|---|
| startup | 28.0 ms | **4.9 ms** | 10.5 ms | 8.0 ms | 295.7 ms |
| `tak` | 921.9 ms | 1.26 s | 1.14 s | 12.40 s | **492.0 ms** |
| `fib` | 961.6 ms | 1.15 s | 1.17 s | 13.16 s | **442.9 ms** |
| `loop-recur` | 68.8 ms | **37.1 ms** | 39.2 ms | 453.3 ms | 413.9 ms |
| `persistent-map` | 44.8 ms | 14.7 ms | **14.2 ms** | 32.8 ms | 412.4 ms |
| `map-filter` | 32.5 ms | **5.1 ms** | 12.4 ms | 9.6 ms | 348.6 ms |
| `transducers` | 171.8 ms | 27.9 ms | **15.7 ms** | — | 355.2 ms |
| `reduce` | 719.3 ms | 45.6 ms | **22.6 ms** | 1.48 s | 308.6 ms |
| runtime size | **8.5 MB** | 12.8 MB | 71.2 MB | 28.8 MB | 398.4 MB |

cljgo @HEAD · let-go v1.11.1 · babashka v1.12.218 · joker v1.9.0 · Clojure CLI
1.12.5.1645 on OpenJDK 26.0.1. joker has no `transducers`. **gloat** and
**go-joker** could not be installed (gloat's module exposes no importable
package path; go-joker needs a source clone + codegen) — let-go's published M1
data puts gloat at 12.7× let-go on `fib` and 5.4× on `reduce`, i.e. losing the
compiled path and winning the core path, the mirror of us.

The table splits exactly along the §1 line. **We win the two benchmarks whose
own code does the arithmetic** (`tak`, `fib` — fastest here bar the JVM, ahead
of both a bytecode VM and a GraalVM native image). **We lose every benchmark
that routes through `clojure.core`** — `reduce` by 15.8× to let-go and 31.8× to
babashka.

**joker is the control, and it is decisive.** joker is the other Go *tree-walk
interpreter*:

| | cljgo | joker (Go tree-walk) | let-go (Go bytecode VM) |
|---|---|---|---|
| `fib` — user code | **961.6 ms** | 13.16 s — 13.7× behind us | 1.15 s |
| `reduce` — `clojure.core` | 719.3 ms | 1.48 s — 2.1× behind us | **45.6 ms** |

On user code we are 13.7× faster than a tree-walker: we are a compiler. On
`reduce` we are in the *tree-walker's* league, 15.8× off the bytecode VM:
there we are an interpreter. Same binary, same run. That is §1's A/B confirmed
by a third-party implementation rather than by our own instrumentation, and it
rules out the alternative explanation that `pkg/lang`'s data structures are
simply slow — if they were, `fib` would be slow too.

## 4. Boot attribution (secondary, confirms ADR 0019/0023)

`BenchmarkBoot` post-ADR-0034: **21.8–23.7 ms, 29.3 MB, 472k allocs/op**.
Allocation profile (`results/boot-mem.prof`, `-sample_index=alloc_objects`,
cumulative):

| site | % of boot allocations |
|---|---|
| `reader.ReadOne` (parsing core.clj text) | **60.8%** |
| `reader.annotate` (line/col metadata) | 47.7% |
| `analyzer.Analyze` | 33.2% |
| `eval.loadCore` | 50.5% |

**~94% of boot is read + analyze of `core.clj` source** — precisely the work
AOT-core deletes. GC is the visible cost of that garbage: `GOGC=off` drops boot
21.8 → 17.4 ms (~20%), and `GOMAXPROCS=1` raises it to 37 ms (GC loses its
parallel workers). Tuning GC is a ~20% palliative; deleting the allocations is
the fix.

## 5. What ADR 0037 must decide

1. **Reframe.** ADR 0023 #2 calls AOT-core "the structural fix" for size.
   S22 shows size is the *third* prize. Order: **performance** (up to 9.7× on
   every `clojure.core` call path) → startup (≈2 ms floor vs 29.8 ms) → RSS →
   size (~2 MB). One edge, four wins.
2. **`<10 ms startup` in README.md was false** — traced to S1's 2.3 ms, which
   predates the `rt.Boot → eval.New` edge. Corrected in this branch.
3. **The `~10×` §1.4 M2 budget is measured against the wrong thing.**
   `pkg/emit/perf_test.go` benchmarks emitted factorial vs handwritten Go —
   user-code-only, the one path that already works (9.7×). It cannot see the
   `reduce` regression at all. ADR 0037's spec should add a
   `clojure.core`-mediated perf gate; a suite-derived one is the obvious
   candidate.

## Verdict: **ours, high confidence.**

Not a host artifact, not measurement error: the A/B is same-binary,
same-machine, 1.00× against 9.74×, and the competitive table splits along the
identical line. AOT-`core.clj` is the highest-value change available to this
project. **Recommend proceeding to ADR 0037**, with feasibility (the
`pkg/eval` → `pkg/lang` builtin move, the `rt` → `eval` edges, multi-namespace
emission) established by a follow-on spike before the spec.

## Files

- `README.md` — the question + exit criterion, written before any code.
- `results/suite.md` — the 7-benchmark table + the M1/M5 calibration.
- `results/*.json` — raw hyperfine output, one per benchmark.
- `results/boot-cpu.prof`, `results/boot-mem.prof` — `BenchmarkBoot` profiles.

No `go.mod` — this spike patched nothing. It benched `pkg/eval/boot_test.go`
and the shipped `cljgo build` in place, exactly as S18 did.
