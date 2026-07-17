# Spike S24 verdict — one native builtin closes 89% of the `reduce` gap

Closed 2026-07-17. Recommendation feeds **ADR 0039** (reserved): hot core fns
become native Go builtins — the pattern every fast Clojure-on-Go already uses,
and the pattern cljgo itself already uses for 292 other fns.

**Exit criterion: MET, decisively.** The bar was ≤ 150 ms on `reduce` (1e6);
measured **82.3 ms**, from 672.2 ms — an 8.2× improvement from moving **one
function** across a line the codebase already draws. The 69-line diff is
frozen as `prototype.patch`.

## 1. The research finding

Every fast Clojure hosts its hot core fns natively; none of them interpret or
even bytecode-compile the `reduce` loop itself:

- **let-go**: `reduce` is handwritten Go —
  `references/let-go/pkg/rt/native_prims.go:332-357` (439 lines of native
  prims). The bytecode VM never runs the fold; Go does.
- **joker**: core fns are native Go (and it is still slow, because
  *everything else* is tree-walked — the control case).
- **babashka**: core is real Clojure compiled by the JVM compiler, then
  GraalVM-native'd. Native again.

cljgo has the identical mechanism — ~292 builtins via `internBuiltins`;
`range` is already one (`pkg/eval/coll_builtins.go:39`) — but `reduce` sat on
the interpreted side (`core/core.clj:543`), so the hottest loop in the
language ran through the tree-walker in both modes.

## 2. Measured (M5 Pro, go1.26.3; hyperfine 3 warmup / 10 runs)

| benchmark (totals incl. startup) | before | after | let-go | gap before → after |
|---|---|---|---|---|
| `reduce` (1e6), binary | 672.2 ms | **82.3 ms** | 46.4 ms | 14.5× → **1.77×** |
| `reduce` (1e6), REPL (`cljgo run`) | ~700 ms | **84.0 ms** | — | dual-mode, same fn |
| `transducers` (1e5) | 154.7 ms | **101.1 ms** | 25.9 ms | 6.0× → 3.9× |
| `persistent-map` (1e4) | 40.1 ms | **33.5 ms** | 13.1 ms | 3.1× → 2.6× |
| `map-filter` | 28.6 ms | 29.1 ms | 5.0 ms | unchanged (see §3) |

Correctness: all benchmark outputs byte-identical; the six-case oracle file
(both arities, empty/nil colls, `reduced` short-circuit, `reduce-kv`) matches
JVM Clojure 1.12.5 exactly; full gates green; **jank suite 217/242 — zero
regressions**.

## 3. Startup-subtracted, the residuals are named

After the fix, cljgo boot is ~28 ms and let-go's ~4.9 ms; subtracting both:

| benchmark | cljgo compute | let-go compute | read |
|---|---|---|---|
| `reduce` | 54.3 ms | 41.5 ms | 1.3× — the `pkg/lang` residual (S23's ~1.76×) |
| `persistent-map` | 5.5 ms | 8.2 ms | **we now win compute** |
| `map-filter` | ~1 ms | ~0.1 ms | boot-dominated; was never a compute loss |
| `transducers` | 73.1 ms | 21.0 ms | 3.5× — the xform closures (`map`/`filter` transducer arities) are still interpreted, called per element |

So after ONE moved fn, the remaining losses decompose into exactly two known
items: the 28 ms boot (ADR 0037's startup prize) and the still-interpreted
lazy-seq/transducer producers (`map`, `filter`, `take`, `transduce` — the
next candidates for the same one-fn treatment).

## 4. Why this is the simple fix (and what it is not)

- **69 lines, one afternoon, 8.2×.** Versus AOT-core: multi-namespace
  emission as a prerequisite, an all-or-nothing 13-file migration, a
  milestone.
- **Incremental** — per-function, each independently gated by oracle + suite.
- **Dual-mode by construction** — one Go fn bound in one var; REPL and binary
  call the same code (design/00 §2's invariant, satisfied trivially). AOT-core
  helps only binaries; this helped the REPL identically (84.0 ms).
- **Precedented, not new architecture** — the codebase already draws this
  line 292 times; JVM Clojure itself does the same (its `reduce` bottoms out
  in Java via `IReduce`/`InternalReduce`, not in interpreted Clojure).
- **It is NOT the whole answer.** It collects none of the startup/RSS/size
  prize (interpreter still linked, `core.clj` still boots), and per-element
  interop through `Apply2` keeps the ~1.3× `pkg/lang` residual. ADR 0037's
  structural fix remains correct long-term; this removes the urgency that
  made it look like the only lever.

## 5. What ADR 0039 must decide

1. **Ratify the pattern**: hot core fns are native builtins; the S22 field
   table is the evidence standard for "hot".
2. **The candidate list, by measured impact**: `reduce` (done here),
   then the seq producers `map`/`filter`/`take` and the transducer arities
   (`transducers` residual 73 ms), `doseq`/`dotimes` loops if profiling
   agrees. Each lands as its own small PR: builtin + core.clj deletion +
   oracle cases + suite run.
3. **Guard the semantics**: every moved fn keeps its oracle comment at the
   builtin, verified against JVM Clojure — the precedence principle applies
   (a native fn may not drift from clojure.core semantics).
4. **Relation to ADR 0037**: complementary, not competing. 0038 takes the
   multiplier now in 69-line steps; 0037 keeps startup/size/structure. The
   `clojure.core`-mediated CI perf gate (ADR 0037 decision #5) should gate
   both.

## Verdict: **the simple fix is real — high confidence.**

One existing-pattern builtin took the worst row of the field table from
14.5× behind let-go to 1.77×, in both modes, with zero suite regressions and
JVM-oracle-identical semantics. Recommend ADR 0039 ratify incremental native
hot-path builtins as the near-term performance strategy, with AOT-core
(ADR 0037) unchanged as the structural goal.

## Files

- `README.md` — question + exit criterion, written before code.
- `prototype.patch` — the 69-line diff (builtin + core.clj deletion),
  **reverted from the working tree after measurement** per ADR 0027;
  production lands via ADR 0039 → OpenSpec.
- `results/s21-reduce.json`, `results/s21-others.json` — raw hyperfine data.

No `go.mod` — the prototype patched the worktree in place and was reverted;
nothing here builds.
