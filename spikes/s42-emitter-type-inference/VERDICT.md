# Spike s42 — emitter numeric type inference (unboxed int64)

> **Outcome (closing note, 2026-07-23):** shipped as **ADR 0067 (accepted,
> owner-directed)** on this branch, integrated with main's #90–#93. The
> open risk below (core-arithmetic redefinition) was resolved by wiring
> every typed region behind the ADR 0066 `rt.CoreDirty()` entry guard —
> proven by `conformance/tests/numeric-redefs-unboxed-paths.clj` (dual
> harness, REPL == compiled). Post-integration numbers and the final rules
> live in the ADR; the text below is the frozen spike record.

**Verdict: WORTH BUILDING.** A conservative int64 inference pass in the
emitter turns the ~35× factorial gap into **5.1×** and removes the 12M
`convT64` allocations the pprof decomposition blamed for ~23% CPU. The
prototype is committed, gates green, dual-harness conformance byte-identical.
Recommend productionizing tiers 1–3 for int64 behind **ADR 0067**, after the
owner ratifies the one semantic decision (core-arithmetic redefinition).

## Exit criteria — answered with evidence

**1. Can the emitter prove a local is int64 and emit raw Go? — YES.**
`inferNumeric` (`pkg/emit/numtype.go`) is a bottom-up fixpoint proving int64
from integer literals, checked arithmetic on int64 operands (`+ - * inc
dec`), `loop`/`recur` carriers seeded numeric, and self-recursive calls.
Emission has three tiers: loop/let carriers → `var i int64` + raw
`rt.IAdd/ISub/IMul` + raw `< > ==`; single-arity fn param specialization
(dual body guarded by `p.(int64)`); and a rung-3 lift of self-recursive
capture-free fns to `func factL(int64) int64` with direct recursion.
Everything uncertain stays boxed.

**2. Measured win — YES** (`go test ./pkg/emit -run TestS42Measure`,
darwin/arm64, best-of-3):

| kernel              | wall off→on | speedup | mallocs off→on |
|---------------------|-------------|---------|----------------|
| loop/recur sum ×10M | 190→14 ms   | 13.9×   | 20,155,450 → 155,708 |
| `(fact 15)` ×2M     | 472→69 ms   | 6.8×    | 24,155,536 → 4,155,758 |
| `fib 35`            | 281→30 ms   | 9.3×    | low in both (add results) |

`TestFactorialPerfBudget`: **35× → 5.1×** vs handwritten Go. The `convT64`
allocations fall exactly where predicted.

Nuance worth stating: **body specialization alone is only ~2×** on
fact/fib because each recursive return re-boxes across the `any` FnFunc
boundary. The alloc win needs the **rung-3 typed-func lift** (direct int64
recursion). Loops get the full win from tier 1 alone (accumulator stays in a
register).

**3. Semantics parity — PRESERVED, byte-identical.** Full dual-harness
conformance (412 files, REPL vs compiled) is green. The unboxed ops
reproduce `int64Ops`' overflow tests and throw the identical
`ArithmeticException("integer overflow")`; `numeric-overflow-throws` /
`numeric-promotion` now execute through `rt.IAdd` and still pass. `/`,
ratios, int/float contagion, BigInt all reach the boxed path (float/BigInt
operands are non-int64; float/BigInt runtime args fail the entry guard).
Overflow can't silently wrap: the checked ops panic exactly as the tower.

**4. Boundary boxing — PROVEN.** `mixedvec`
(`(loop [i 0 v []] … (conj v (* i i)))`) unboxes `i`/`(* i i)` and boxes
correctly into the vector → `[0 1 4]`. `floatstay` keeps a float accumulator
boxed while unboxing the int counter → `4.5`. A non-int64 caller of a
specialized fn (`(f 3.0)`, or reduce-kv passing a keyword) takes the boxed
fallback.

## What it infers vs punts on

Infers: int64 loop/let/method carriers; single-fixed-arity int64 fn params;
self-recursive int64 returns; `+ - * inc dec < > =`.
Punts (stays boxed): float64 (all of it); `/`, ratios, BigInt; multi-arity /
variadic / >4-arity fns; capturing closures (rung-3 lift); cross-fn (non-
self) return typing; any other op.

## Un-proven risks

1. **Core-arithmetic redefinition.** The unboxed path ignores
   `(with-redefs [+ …])`, matching JVM's primitive intrinsics but STRICTER
   than cljgo's boxed intrinsic (which keeps ADR-0004 liveness). No
   conformance test exercises it, so the harness stays green — but it is a
   real narrow divergence. **Owner must ratify** in ADR 0067, or add the
   `rt.ArithPristine()` once-per-call entry guard (keeps the boxing win).
2. **float64 unhandled** — a large fraction of numeric code sees no benefit.
3. **`rt.MustInt64`** on the inline (non-lifted) self-call is an inference
   invariant; sound today, but it is a place a future inference bug would
   surface (routed through the error channel, never a raw crash).
4. **rung-3 lift scope** is capture-free / no-nested-fn only.

## Recommended apply scope (v1)

Ship tiers 1–3 for **int64** behind ADR 0067. Ratify the redefinition
semantics (or add `ArithPristine`). Follow-ups: float64, multi-arity
specialization, broadening the lift. Keep `CLJGO_NUMINFER_OFF` as a kill
switch. This needs to be behind ADR 0067 before applying — it changes
emitted-code shape and carries the one semantic decision above.

## Files

- `pkg/emit/numtype.go` — the inference pass (+ `CLJGO_NUMINFER_OFF` toggle).
- `pkg/emit/rt/inttypes.go` — `IAdd/ISub/IMul/IInc/IDec/MustInt64`.
- `pkg/emit/emit.go` — tiered emission (genIntrinsic / genTestIntrinsic /
  specializeInt / emitTypedGuard / emitTypedFunc / genSelfCallInt).
- `pkg/emit/program.go` — lifted-func emission + env-gated alloc report.
- `pkg/emit/numtype_test.go`, `pkg/emit/s42_measure_test.go` — regression +
  A/B measurement.
- `pkg/coreaot/*` — regenerated with the pass active.
