## 1. pkg/lang surgery (vendored — log in PROVENANCE.md)

- [x] 1.1 Rewrite `pkg/lang/bigdecimal.go`: unscaled `*big.Int` + `int32`
  scale, ported from the S16 prototype — parse (Java ctor grammar),
  FromInt64/FromBigInt (scale 0), FromFloat64 (valueOf semantics; Inf/NaN
  throw), FromRatio (exact divide), Add/Sub (scale max), Mul (scale sum),
  Divide (exact-or-throw, preferred scale sx−sy), Quotient
  (divideToIntegralValue preferred-scale rules), Remainder, Cmp,
  Negate/Abs, javadoc toString, StripTrailingZeros, hasheq via the
  stripped value. Keep exported names; drop the big.Float members.
  Gates green.
- [x] 1.2 `pkg/lang/numberops.go`: bigDecimalOps rows re-targeted
  (Sign-based IsPos/IsNeg/IsZero; Divide/Quotient/Remainder to the new
  methods); `AsBigDecimal` — ints scale-0 direct, float64 via
  FromFloat64 (throws Infinite or NaN), BigInt/big.Int/Ratio exact.
  Verify the Combine matrix rows unchanged. Gates green.
- [x] 1.3 Ripple arms: `pkg/lang/numbers.go` (AsInt64, AsFloat64,
  Rationalize), `pkg/lang/bigint.go` ToBigDecimal,
  `pkg/lang/strconv.go` printer (String()+"M" readably; String() for
  str). Gates green.
- [x] 1.4 PROVENANCE.md entry citing ADR 0032 + S16 evidence.

## 2. Reader / emitter / eval wiring

- [x] 2.1 `pkg/reader/number.go`: M literal flows through the new parser
  (scale + E-notation preserved; long literals exact). Gates green.
- [x] 2.2 `pkg/emit/emit.go`: `MustBigDecimal(x.String())` round-trip —
  covered by dual-harness conformance files; add/verify a unit
  round-trip test. Gates green.
- [x] 2.3 `pkg/eval/numeric_builtins.go`: `bigdec` (string parse via the
  new grammar; nil still throws), `rationalize` BigDecimal arm exact.
  Gates green.

## 3. Conformance + evidence

- [x] 3.1 S16 corpus → grouped `conformance/tests/bigdec-*.clj`
  (literals, coercion, arithmetic, division, equality/hash, printing,
  tower) with frozen oracle-cited `;; expect:` output, re-verified rows
  against the real `clojure` CLI at freeze time. Dual harness green.
- [x] 3.2 `cljgo suite` before/after recorded; `bigdec.cljc` fail → pass.
- [x] 3.3 Full gates: `go build ./... && go vet ./... && gofmt -l pkg cmd
  conformance && go test ./...` green.
