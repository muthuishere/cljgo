# S16 — a real scaled-decimal BigDecimal

## Question

cljgo's `BigDecimal` (`pkg/lang/bigdecimal.go`) is backed by `big.Float`:
it has **no scale** (trailing zeros are lost — `1.10M` prints `1.1M`, and
`(= 1.0M 1.00M)` can't be answered the Java way), no exponential-notation
fidelity (`(bigdec "1e10")` should print `1E+10M`), and it *can* represent
`##Inf`/`##NaN`, which `java.math.BigDecimal` cannot. ADR 0029 deferred
this as cluster C of spike S13; it blocks the suite's `bigdec.cljc` /
`with_precision.cljc` files and S14's deferred `%f`-on-BigDecimal tail.

**What is the right scaled-decimal representation on Go, and what does
the migration cost?**

Candidates:

- **(a)** unscaled `*big.Int` + `int32` scale — Java BigDecimal's exact
  model; we implement arithmetic/rounding/toString ourselves. Prototyped
  here.
- **(b)** an existing pure-Go decimal library (shopspring/decimal,
  cockroachdb/apd). cljgo has ZERO external deps (a standing constraint);
  assessed on paper only.
- **(c)** keep `big.Float`, carry a separate scale for printing only —
  cheapest, but is arithmetic still correct? Assessed on paper.

## Method

- `probes.clj` — one labeled probe per behavior, runnable unmodified by
  both `clojure -M` (the oracle, real Clojure 1.12.5) and
  `go run ./cmd/cljgo run` (current cljgo, the divergence baseline).
  Rows: every S13 cluster-C divergence, every assertion in the suite's
  `bigdec.cljc` and `with_precision.cljc`, plus a systematic sweep of
  literal scale preservation, arithmetic scale rules (add/sub = max
  scale, mul = sum, div preferred scale / non-terminating throw),
  `with-precision` MathContext rounding modes, `=` vs `==` vs `compare`
  scale sensitivity, Java's plain-vs-E-notation toString boundary, and
  tower interaction (`(+ 1 1.0M)`, `(/ 1M 3)`, `(+ 1.0 1.0M)` …).
- `run_probes.sh` — runs both hosts, writes `out/probes.oracle.txt` and
  `out/probes.cljgo.txt`.
- `proto/` (own `go.mod`, fenced from the main module like every spike) —
  prototype of candidate (a): `decimal.go` implements unscaled
  `big.Int` + scale with Java's arithmetic scale rules, MathContext
  division/rounding (all 8 RoundingModes), and the `BigDecimal.toString`
  plain-vs-scientific algorithm ported from its javadoc. `main.go` is a
  harness: it re-computes every probe through the prototype and diffs
  against `out/probes.oracle.txt` line by line.
- Migration inventory — grep of every place `BigDecimal` flows in
  `pkg/` + `core/`, each touchpoint with the change it needs (VERDICT).

## Exit criterion

Written before any code, per ADR 0027:

1. An oracle-verified probe corpus (every row frozen from real Clojure
   1.12.5) covering the categories above, with the current-cljgo
   divergence count recorded as the baseline.
2. The candidate-(a) prototype passes the corpus — target ≥ 95% of rows
   it implements, every miss explained — proving the representation is
   buildable without external deps.
3. A migration-cost inventory: every `pkg/`/`core/` touchpoint listed
   with the change it needs, sized, so ADR 0032 is schedulable.
4. `VERDICT.md` recommends one representation for ADR 0032.

This spike does not change `pkg/`. Prototype code never merges; it only
informs.
