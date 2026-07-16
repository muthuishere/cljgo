## 1. pkg/lang surgery (vendored — log in PROVENANCE.md)

- [x] 1.1 Cluster A: rewrite `float64Ops.Quotient`/`Remainder`
  (pkg/lang/numberops.go) to the JVM `Numbers.quotient/remainder(double,
  double)` shape: `Divide by zero` on 0.0 divisor, `Infinite or NaN` when the
  quotient is non-finite, double result from the big-integer truncation
  otherwise. Gates: `go build ./... && go vet ./... && gofmt -l pkg cmd
  conformance && go test ./...` green.
- [x] 1.2 Cluster B: `AsBigInt(float64/float32)` converts via the
  shortest-round-trip decimal representation truncated toward zero
  (BigDecimal.valueOf semantics, oracle-verified); Inf/NaN throw `Infinite
  or NaN`. Gates green.
- [x] 1.3 Cluster F: `IntCast` (pkg/lang/numbers.go) bounds against int32:
  integral overflow → `integer overflow`; double out of range → `Value out of
  range for int: <Double.toString>`; BigInt beyond int64 → `Value out of
  range for long: <n>`. Gates green.
- [x] 1.4 PROVENANCE.md entry citing the oracle outputs for 1.1–1.3.

## 2. Wiring

- [x] 2.1 Cluster E: register `abs` (lang.Abs dispatcher) as a clojure.core
  builtin in pkg/eval. Gates green.
- [x] 2.2 Cluster D: `even?`/`odd?` in core/core.clj guard `(integer? n)` and
  throw `Argument must be an integer: <n>` via `-illegal-argument`. Gates
  green.

## 3. Conformance + evidence

- [x] 3.1 New conformance/tests/*.clj (dual-harness) with frozen,
  oracle-cited expectations: float quot/rem/mod Inf/NaN/zero guards, bigint
  of large doubles, even?/odd? guard, abs surface, int 32-bit bounds. Gates
  green.
- [x] 3.2 Re-run the S13 probes (spike dir untouched — copy the runner) and
  record the divergence drop; run `cljgo suite` before/after and record the
  file counts. Gates green.
