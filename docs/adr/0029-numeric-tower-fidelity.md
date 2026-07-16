# ADR 0029 — Numeric tower fidelity: fix the cheap clusters, defer BigDecimal
Date: 2026-07-16 · Status: accepted · Evidence: spike S13 (spikes/s13-numeric-divergences/VERDICT.md)

## Context

S13 probed 276 op×type cases against real Clojure 1.12.5: 99 diverge,
clustering into 7 root causes with exact pkg/lang citations.

## Decision

Fix clusters A, B, D, E, F — guard-clause-sized, they close the suite's
quot/mod/rem/even?/odd?/abs/int buckets (~12 files):

- **A**: float64Ops Quotient/Remainder (numberops.go) guard ##Inf/##NaN.
- **B**: AsBigInt(float64) converts exactly instead of saturating int64(x).
- **D**: even?/odd? (core.clj) get the integer-type guard (throw on non-int).
- **E**: wire `abs` — every per-type Ops.Abs already exists.
- **F**: IntCast bounds against Java's 32-bit int range, not Go's platform int.

**Deferred, own future ADR**: cluster C — BigDecimal is backed by big.Float
(no scale, loses trailing zeros, can represent Inf/NaN); a real
scaled-decimal representation is a structural change, not a guard fix.
**Out of scope**: cluster G — exception-message wording (both hosts throw;
the suite's thrown? assertions don't read messages).

## Consequences

The suite's numeric bucket closes without destabilizing the tower.
BigDecimal fidelity (and with-precision) stays an honest, recorded gap.
