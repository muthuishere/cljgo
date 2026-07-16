# ADR 0032 — BigDecimal is an unscaled big.Int + int32 scale
Date: 2026-07-16 · Status: accepted · Evidence: spike S16 (spikes/s16-bigdecimal-scaled/VERDICT.md) · Completes ADR 0029's deferred cluster C

## Context

cljgo's BigDecimal is backed by big.Float: no scale (trailing zeros lost),
Inf/NaN representable (Java's cannot be), and — decisively — it SILENTLY
CORRUPTS long literals via mantissa truncation. S16's 159-row oracle corpus
shows current cljgo diverging on 72/132 comparable rows; the arithmetic
itself is binary-wrong, so a print-only scale fix is insufficient. External
decimal libraries were rejected: the zero-external-deps constraint stands,
and both candidates would still need a Java-semantics wrapper.

## Decision

Adopt S16's candidate (a): **unscaled `*big.Int` + `int32` scale** — Java
BigDecimal's exact model, ~450 lines, stdlib only, measured **159/159**
against real Clojure 1.12.5. Includes: Java's arithmetic scale rules
(add/sub max, mul sum, div exact-or-throw), all 8 RoundingModes via exact
true-remainder comparison, the javadoc toString plain-vs-E algorithm, and
the oracle-pinned equality semantics — `(= 1.0M 1.00M)` is TRUE
(compareTo-based; hasheq strips trailing zeros), cross-category
`(= 1M 1)` / `(= 1M 1N)` are false.

Migration: the 14-touchpoint inventory in the VERDICT (items 1–8 = one
coherent representation swap; `with-precision` + format `%f`-on-BigDecimal
follow on). Vendored-file surgery logs in pkg/lang/PROVENANCE.md.

## Consequences

Closes the suite's bigdec/with-precision cluster and S14's deferred format
tail. Kills a silent data-corruption bug.
