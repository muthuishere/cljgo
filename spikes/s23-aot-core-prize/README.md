# Spike S23 — How big is the AOT-core prize, and what blocks collecting it?

Opened 2026-07-16. Feeds **ADR 0037** (reserved), together with S22.

## Context

S22 established that `cljgo build` compiles the user's forms and leaves
`clojure.core` as interpreted tree-walk closures: 9.74× on user code, **1.00×**
on `clojure.core` work. It did not establish **how much** AOT-compiling
`core.clj` would recover, nor what it costs to build.

Guessing the payoff from S22 is not sound: the 9.74× user-code figure is
measured on `fib`, which is arithmetic through `rt`'s guarded intrinsics
(`rt.Add2`/`Sub2`, ADR 0004) — the most favourable possible path. `reduce` is
seq traversal and megamorphic `IFn` dispatch. The prize must be measured on
`reduce`'s actual shape, not extrapolated from `fib`'s.

## The one question

**If `clojure.core/reduce` were compiled Go instead of an interpreted closure,
how much of the 16.54× let-go gap would close — and is the change feasible
without breaking the one-analyzer/two-backends contract?**

## Exit criterion (written before any code, per ADR 0027)

**Part A — the prize (measurement).** Implement `reduce`'s exact algorithm in
Clojure *in the user file*, so `cljgo build` compiles it, and race it against
interpreted `clojure.core/reduce` over the identical input. Both consume the
same (interpreted, lazy) `core/range`, so the delta isolates reduce-the-fn.

- **If compiled `my-reduce` ≥ 3× faster than core's `reduce`** → the prize is
  real and large; the `reduce` gap is the interpreted-core defect, not the
  runtime's seq/`IFn` machinery. Proceed to Part B.
- **If compiled `my-reduce` < 1.5× faster** → the bottleneck is `pkg/lang`
  (seqs, boxing, `IFn` dispatch), NOT interpretation. AOT-core would be a
  startup/size win only, ADR 0023's original framing was right, S22's
  performance reframing is **wrong**, and the gap needs its own spike.
- Between 1.5× and 3×: report both contributions; ADR 0037 must schedule the
  runtime work alongside AOT-core rather than instead of it.

**Part B — the cost (feasibility inventory).** Enumerate, with file:line, every
edge that must be severed for the Go linker to drop `pkg/eval` from an emitted
binary, and state for each whether it is mechanical or a design question.
Exit criterion: a written inventory an ADR can cost, not a prototype.

Part B is inventory-only by design. A full AOT-core prototype is multi-week
(the ~292 builtins in `pkg/eval` must move to `pkg/lang`; `pkg/emit/rt` imports
`pkg/eval` for interop/throw; multi-namespace emission does not exist) and must
not be started before ADR 0037 rules on the approach.

## Method

Host: Apple M5 Pro, go1.26.3. hyperfine, 3 warmup / 10 runs, per S22.
Corpus: `references/let-go/benchmark/reduce.clj` as the baseline.

## Results

See `VERDICT.md`. Raw data in `results/`.
