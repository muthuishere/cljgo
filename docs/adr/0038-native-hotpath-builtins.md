# ADR 0038 — Hot core fns are native Go builtins (the simple fix, taken first)
Date: 2026-07-17 · Status: proposed
Complements: **ADR 0037** (AOT-core, structural) — does not supersede it.
Evidence: spike **S21** (`spikes/s21-native-core-hotpath/VERDICT.md`),
building on S19/S20.

## Context

S19/S20 proved `clojure.core` is tree-walk-interpreted in both modes and
priced the structural fix (AOT-core: ~86% of the gap, gated on multi-namespace
emission, all-or-nothing, a milestone). Before starting big work we researched
how the fast implementations actually do it, and the answer is uniform:
**nobody interprets — or even bytecode-compiles — the hot core loops.**
let-go's `reduce` is handwritten Go (`pkg/rt/native_prims.go`, 439 lines of
native prims); joker's core fns are native Go; babashka's core is
GraalVM-compiled. JVM Clojure itself bottoms `reduce` out in Java
(`IReduce`/`InternalReduce`).

cljgo already draws this exact line 292 times (`internBuiltins`; `range` is a
builtin). The defect is only that the hottest fns sat on the wrong side of it:
`reduce` was interpreted Clojure at `core/core.clj:543`.

S21 moved `reduce` alone — a 69-line diff using the existing pattern:

| | before | after | let-go |
|---|---|---|---|
| `reduce` (1e6), binary | 672.2 ms | **82.3 ms** | 46.4 ms |
| `reduce` (1e6), REPL | ~700 ms | **84.0 ms** | — |
| `transducers` (1e5) | 154.7 ms | 101.1 ms | 25.9 ms |
| `persistent-map`, compute (boot-subtracted) | 12.1 ms | **5.5 ms** | 8.2 ms — we now win |

Zero suite regressions (217/242 unchanged), six-case oracle byte-identical to
JVM Clojure 1.12.5 including the `reduced` short-circuit, both modes improved
identically because both call the same Go fn.

## Decision

1. **Ratify the pattern.** Core fns that dominate real workloads are
   implemented as native Go builtins, exactly like the existing 292. This is
   not new architecture; it is placing fns on the correct side of an existing
   line, precedented by JVM Clojure itself.

2. **Simplicity discipline — move only what measurement names.** No bulk
   migration of core.clj. A fn moves when a field-table or profile row blames
   it, one fn per PR: builtin + `core.clj` deletion + oracle cases at the
   builtin + full gates + suite run. The S21 evidence standard (JVM oracle
   match, zero suite regressions, both modes measured) is the bar for each.

3. **Initial candidate list, by measured impact** (each still gated by its
   own before/after): `reduce` (proven in S21 — land it first, it is the
   frozen `prototype.patch`), then the `transducers` residual (73 ms compute
   vs let-go's 21 ms: the transducer arities of `map`/`filter` and
   `transduce`'s loop), then the lazy producers (`map`/`filter`/`take`) if
   profiling still blames them afterwards. Stop when the field table says
   stop — not every fn in core.clj, ever.

4. **Semantics are non-negotiable.** Every moved fn keeps its `;; oracle:`
   citations at the builtin, verified against the real `clojure` CLI. The
   precedence principle applies in full: a native fn may not drift from
   clojure.core behavior; a semantic mismatch is a release blocker like any
   conformance failure.

5. **Relation to ADR 0037: complementary, sequenced.** 0038 takes the
   multiplier now, in 69-line steps that also speed up the REPL. 0037 keeps
   the structural prizes (startup ~2 ms floor, RSS, ~2 MB binaries, dropping
   `pkg/eval` from emitted binaries) and is unchanged — native builtins live
   in `pkg/eval` today, so each move is also one fewer interpreted form for
   the eventual AOT-core migration to carry. The `clojure.core`-mediated CI
   perf gate (0037 decision #5) gates both efforts and should land with the
   first fn.

## Consequences

- The worst field-table row (15.8× behind let-go) drops to 1.77× for one
  small PR, in both modes, this week — instead of after a milestone.
- `core.clj` shrinks slightly; each move also trims boot (fewer forms to
  read/analyze).
- The `pkg/lang` per-element residual (~1.3× on `reduce` compute) remains and
  is the doc 04 §5 ladder's problem — out of scope here, tracked, not urgent.
- Risk: builtin drift from Clojure semantics. Mitigated by decision 4 and the
  dual-harness suite; the S21 `reduced`-box case is the template.
- Risk: creep ("move everything"). Mitigated by decision 2 — measurement
  names the fn, or it does not move.
