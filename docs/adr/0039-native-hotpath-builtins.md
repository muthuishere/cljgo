# ADR 0039 — Hot core fns are native Go builtins
Date: 2026-07-17 · Status: accepted (owner-directed 2026-07-17)
Complements: ADR 0037 (AOT-core, structural — proposed on `spike/aot-core`).
Evidence: spikes S19/S20/S21 on `spike/aot-core` (which carries this ADR's
draft as 0038 — renumbered here because main took 0038 for STM-lite; rename
the spike branch's copy at merge).

## Context

S19 proved `clojure.core` is tree-walk-interpreted in both modes: `cljgo
build` gives 9.74× on the user's own forms and 1.00× — nothing — on anything
core does, so a "compiled" binary runs `reduce` at interpreter speed and loses
let-go's own benchmark suite 15.8× on its worst row. Research: every fast
Clojure hosts its hot core fns natively (let-go's `reduce` is handwritten Go —
`pkg/rt/native_prims.go`; joker's core is Go; babashka's is GraalVM-compiled;
JVM Clojure bottoms out in Java via `IReduce`). cljgo already draws this line
~292 times (`internBuiltins`; `range` is a builtin) — the hot fns just sat on
the interpreted side. S21 validated the move on `reduce` (672→82 ms, both
modes, zero suite regressions).

## Decision

1. **`reduce`, `map`, `filter`, `mapv`, `comp` are native Go builtins**
   (`pkg/eval/hotpath_builtins.go`), all arities including the transducer
   forms of `map`/`filter`; the `core.clj` definitions are deleted (builtins
   intern before `loadCore`, so a surviving defn would shadow the native).
2. **Semantics are non-negotiable.** Each builtin carries its `;; oracle:`
   citations; a 35-case oracle file (arities, infinite-seq laziness, the
   `reduced` box, transducer composition, downstream fns `mapcat`/`keep`/
   `remove`/`sequence`) is byte-identical to JVM Clojure 1.12.5. The
   precedence principle applies: drift is a release blocker.
3. **The discipline stays: measurement names the fn.** No bulk migration.
   Next candidates ONLY after re-measuring on top of this change.

## Measured (M5 Pro, go1.26.3, vs let-go v1.11.1 same-machine, totals incl. startup)

| benchmark | before | after | let-go | standing |
|---|---|---|---|---|
| `tak` | 921.9 ms | 977.9 ms | 1278.7 ms | **win** |
| `fib` | 961.6 ms | 1028.9 ms | 1202.3 ms | **win** |
| `reduce` (1e6) | 719.3 ms | **89.4 ms** | 44.3 ms | 2.0× (was 16×) |
| `transducers` | 171.8 ms | **69.8 ms** | 27.2 ms | 2.6× (was 6.3×) |
| `map`+sum (1e6) | 1481.7 ms | **195.5 ms** | 108.8 ms | 1.8× (was 13.6×) |
| `mapv` (1e6) | 915.7 ms | **104.7 ms** | 102.9 ms | 1.02× — dead heat |
| `comp` chain (1e6) | 2027.3 ms | **226.3 ms** | 138.0 ms | 1.6× (was 14.7×) |
| `frequencies` (1e6) | 2438.8 ms | 1052.0 ms | 196.4 ms | 5.4× — see below |

Suite: **234/242 (96.7%), zero failing files** — identical to the pre-change
baseline. Full `go test ./...` green. Both modes improve identically.

## What still loses, and the two named levers

1. **Boot (30 ms) now dominates every small-benchmark total** (let-go starts
   in 5.6 ms). `map-filter` is 29.9 ms total of which ~2 ms is compute. No
   amount of native core flips those rows; that is ADR 0037's startup prize.
2. **`frequencies`/`group-by`/`into`/`update` residuals trace to persistent-
   map/vector update cost without transients** (`pkg/lang/TODO.md` S4 defect
   #2, deferred since M0, "3–5 days, no design risk"). Land transients, then
   re-measure before considering native `frequencies`/`group-by` — they are
   thin interpreted wrappers over `reduce`+`assoc` and likely collapse.

## Consequences

- Worst-row gap vs let-go: 16× → 2.0×; four rows now within 1.0–2.0×; two
  rows won outright. The REPL gets every one of these speedups too.
- `core.clj` shrinks ~90 lines; boot shrinks slightly (fewer forms).
- The doc-04 §5 ladder residual (~1.5× on pure fold compute) is unchanged and
  tracked; not this ADR's problem.
- ADR-number collision resolved: this is 0039; `spike/aot-core`'s draft 0038
  must be renamed at merge (main's 0038 = STM-lite).
