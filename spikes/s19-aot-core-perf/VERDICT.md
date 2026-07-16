# Spike S19 verdict — `cljgo build` compiles your code and nothing else

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

All 7 files run on cljgo with no edits. Normalized to let-go = 1.00×
(their table's convention; cancels M1-Pro-vs-M5-Pro — calibration in
`results/suite.md`, a tight 1.39–1.85× band, median 1.72×). Lower is faster.

| Benchmark | cljgo | let-go | babashka | joker | go-joker | gloat | fennel | JVM |
|---|---|---|---|---|---|---|---|---|
| `tak` | **0.74×** | 1.00× | 0.9× | — | 0.8× | 10.3× | 5.1× | 0.3× |
| `fib` | **0.82×** | 1.00× | 0.9× | 9.5× | 0.7× | 12.7× | 0.9× | 0.3× |
| `loop-recur` | 1.80× | 1.00× | 1.0× | 10.5× | 0.2× | 15.5× | 2.6× | 6.9× |
| `persistent-map` | 3.09× | 1.00× | 0.9× | 2.5× | 1.0× | 1.6× | 180× | 24.9× |
| `map-filter` | 5.98× | 1.00× | 2.4× | 1.6× | 1.8× | 8.8× | 141× | 49.6× |
| `transducers` | 6.56× | 1.00× | 0.6× | — | 0.4× | 4.3× | 36.4× | 8.3× |
| `reduce` | 16.54× | 1.00× | 0.5× | 37.0× | 0.2× | 5.4× | 121× | 5.5× |
| startup | 6.08× | 1.00× | 2.2× | 1.4× | 1.5× | 1.8× | 5.2× | 43.9× |

The table splits exactly along the §1 line. **We win the two benchmarks whose
own code does the arithmetic** (`tak`, `fib` — fastest in the field bar the
JVM). **We lose every benchmark that routes through `clojure.core`.**

Against **gloat**, the only other Clojure→Go AOT compiler, the same split
appears in our favour where it counts: 12.5× faster on `fib`, 13.9× on `tak`,
8.6× on `loop-recur` — but gloat beats us 3.1× on `reduce` and 1.5× on
`transducers`, i.e. exactly where our core is interpreted and theirs is not.
That is independent corroboration that the emitter is sound and the boot model
is the defect.

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
   S19 shows size is the *third* prize. Order: **performance** (up to 9.7× on
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
