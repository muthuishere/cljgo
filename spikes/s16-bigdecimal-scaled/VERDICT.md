# S16 VERDICT — a real scaled-decimal BigDecimal

**Recommendation for ADR 0032: candidate (a) — unscaled `*big.Int` +
`int32` scale, implemented in-repo on the stdlib (`math/big`), replacing
the `big.Float` inside `pkg/lang/bigdecimal.go`.** The prototype
(`proto/`, ~450 lines of Go, zero external deps) matches the oracle on
**159/159** corpus rows, including every S13 cluster-C divergence, every
assertion in the suite's `bigdec.cljc` and `with_precision.cljc`, all 8
MathContext rounding modes, and Java's plain-vs-E-notation toString
boundary. The migration is 14 touchpoints, most mechanical
(`out/proto_report.txt` is the harness evidence).

## What was measured

### Corpus (159 oracle-verified rows)

- `probes.clj` — 133 rows: literal scale preservation, `bigdec` coercion
  from every tower type + strings + Inf/NaN, arithmetic scale rules,
  exact division, quot/rem/mod, `=`/`==`/`compare`/hash scale semantics,
  printing, tower interaction. Frozen from real Clojure 1.12.5
  (`out/probes.oracle.txt`).
- `probes_wp.clj` — 26 rows: the suite's 18 `with_precision.cljc`
  assertions + default-rounding division + MathContext on add/sub/mul.
  Separate file because `with-precision` doesn't resolve in cljgo (the
  compile error would abort every later form).

### Baseline: current cljgo (big.Float) vs oracle

**72 of 132 comparable rows diverge** (`out/probes.cljgo.txt`), plus
`hash` fails to resolve (aborts the last row), plus **all 26
with-precision rows fail to load**. Divergence families, all reducible
to the representation:

| family | example | oracle | cljgo today |
|---|---|---|---|
| scale lost | `1.10M` | `1.10M` | `1.1M` |
| E-notation lost | `1E10M` | `1E+10M` | `10000000000M` |
| binary artifacts | `(+ 1.1M 2.2M)` | `3.3M` | `3.3000000000000000002M` |
| Inf/NaN representable | `(bigdec ##Inf)` | THREW `Infinite or NaN` | `+InfM` |
| `-0` representable | `(bigdec -0.0)` | `0.0M` | `-0M` |
| div doesn't throw | `(/ 1M 3M)` | THREW non-terminating | `0.33333333333333333334M` |
| div by zero | `(/ 1M 0M)` | THREW `Divide by zero` | `+InfM` |
| huge-double blowup | `(bigdec 1.5e300)` | `1.5E+300M` | 301-digit plain integer `M` |
| precision cap | `123456789012345678901234567890.12M` | exact | `...345678900000000000M` (mantissa truncated) |
| `with-precision` | all 26 rows | values | unable to resolve symbol |

Note the last one: `big.Float` default precision even **corrupts big
literals** — this is silent wrong-answer territory, worse than the
cosmetic trailing-zero loss.

### Prototype: candidate (a) vs oracle

**159/159 (100%)**. 4 rows are THREW-vs-THREW with host-specific wording
(`bigdec nil/true/:a/""` — same scoring policy as S13/S14; the suite's
`p/thrown?` doesn't read messages). Everything the representation itself
answers — scale arithmetic (add/sub = max, mul = sum, div preferred
scale `sx−sy` with padding, `divideToIntegralValue` preferred-scale
rules that make `(quot 10.0M 3)` → `3.0M` but `(quot 10.0M 3.0M)` →
`3M`), the non-terminating-expansion throw (denominator must reduce to
2^a·5^b), all 8 rounding modes with exact true-remainder decisions (no
guard digits), and the toString javadoc algorithm (plain iff
`scale >= 0 && adjustedExponent >= -6`) — is computed by the prototype,
not hardcoded. Tower-dispatch rows (which type wins `+`, what `bigdec`
does on nil) are encoded in the harness since that logic lives in
pkg/lang's Ops matrix, not in the representation.

Oracle findings worth pinning in ADR 0032 (two contradict folklore):

1. **`(= 1.0M 1.00M)` is TRUE** in Clojure 1.12.5 (and `(= (hash 1.0M)
   (hash 1.00M))` is true, `(hash-set 1.0M 1.00M)` has 1 element).
   Clojure `=` on two BigDecimals is equiv/compareTo-based; Java
   `.equals` scale-sensitivity does NOT leak into `=`. Hasheq must
   normalize via stripTrailingZeros. (The task brief assumed FALSE —
   the oracle disagrees; suite row `(= 123…890.0M (bigdec 123…890N))`
   confirms.)
2. Cross-category `=` is false even when values are equal:
   `(= 1M 1)`, `(= 1M 1N)`, `(= 1M 1.0)`, `(= 0.5M 1/2)` all false;
   `==` true for each.
3. `(/ 1M 3)` throws exactly like `(/ 1M 3M)` (long promotes to scale-0
   BigDecimal first); `(/ 1M 0M)` throws `Divide by zero`, never Inf.
4. `(+ 1.0 1.0M)` → `2.0` (double contaminates; BigDecimal never holds
   Inf/NaN because those additions leave the decimal category).
5. `(with-precision 2 (+ 123M 0M))` → `1.2E+2M` — MathContext rounding
   can produce negative scales; the printer must handle them.

## Representation candidates

### (a) unscaled `*big.Int` + `int32` scale — RECOMMENDED

Java BigDecimal's exact model, so every javadoc rule ports 1:1 and the
oracle stays checkable forever. Proven above at 100% corpus match in
~450 lines with only `math/big`. Immutable by construction (same
discipline the current wrapper claims). No new deps, no vendoring, no
license surface. Cost: we own arithmetic + rounding + toString — but the
prototype IS that code; the hard parts (preferred-scale division,
true-remainder rounding, toString boundary) are already written and
oracle-verified.

### (b) external decimal library — REJECTED

- `shopspring/decimal` (MIT): value `*big.Int` + `int32` exponent — the
  same model as (a) — but its API normalizes/strips in places, its
  division defaults to a fixed `DivisionPrecision` (16) rather than
  Java's exact-or-throw, and it has no MathContext/RoundingMode surface;
  we'd wrap and fight it.
- `cockroachdb/apd` (Apache-2.0): closest semantic fit (coefficient +
  exponent + Context with precision/rounding, GDA-compliant), but it's
  a General Decimal Arithmetic implementation, not a Java BigDecimal —
  preferred-scale rules, toString switch-over, and the exact-divide
  throw all differ and would need a Java-semantics wrapper anyway.

Both violate the standing **zero-external-deps** constraint (go.mod
today has only `golang.org/x/tools`); vendoring ~5-10k lines plus
license headers to still need a semantics wrapper is strictly worse
than owning the 450 proven lines.

### (c) big.Float + carried scale for printing — REJECTED

Fixes only the trailing-zero rows. Arithmetic stays binary: `(+ 1.1M
2.2M)` still yields `3.3000000000000000002M`, big literals still get
mantissa-truncated (silent corruption), Inf/NaN stay representable,
exact-divide can't throw on non-termination (the information isn't
there), and `with-precision` (decimal significant digits) can't be
implemented on a binary mantissa. The corpus proves the arithmetic is
wrong, not just the printing — (c) cannot exit this spike.

## Migration inventory (what ADR 0032's spec must schedule)

Fourteen touchpoints, verified by grep over `pkg/` + `core/`:

| # | file:line | today | change | size |
|---|---|---|---|---|
| 1 | `pkg/lang/bigdecimal.go` (whole file) | wraps `*big.Float` | replace with unscaled `*big.Int` + `int32` scale; port `proto/decimal.go` (Parse, FromX, Add/Sub/Mul, Divide/DivideToIntegral/Rem, Round/DivideMC, String, StripTrailingZeros); keep the exported names (`NewBigDecimal`, `MustBigDecimal`, `NewBigDecimalFrom*`, `ToBigInteger`, `ToBigFloat`, `Cmp`, …) so most callers don't move | **L** (the core; ~450 proven lines exist) |
| 2 | `pkg/lang/numberops.go:519-600` | `bigDecimalOps` methods reach into `.val` (`big.Float`) | re-target to the new API; `Divide` must throw non-terminating/zero; `Quotient`/`Remainder` use the new preferred-scale methods; `Equiv` stays Cmp-based (oracle finding 1) | M |
| 3 | `pkg/lang/numberops.go:940-985` (`AsBigDecimal`) | int/int64/uint64 → `float64` → big.Float (**precision loss for \|x\| > 2^53 today**) | ints → scale-0 unscaled directly; float64 via valueOf semantics (shortest string), throwing on Inf/NaN — closes `(bigdec ##Inf)` | S |
| 4 | `pkg/lang/numberops.go` Combine matrix (~724-790) | routes to bigDecimalOps | unchanged logic; verify `(+ 1.0 1.0M)` → float64Ops still wins | S (verify only) |
| 5 | `pkg/lang/strconv.go:292` (print readably) | `StripTrailingZeros()` + `"M"` — the `1.10M`→`1.1M` bug site | print `String()` (Java toString) + `"M"`; `strconv.go:104` (`str`) prints `String()` without suffix | S |
| 6 | `pkg/reader/number.go:79` (M literal) | `NewBigDecimal(m[1])` via big.Float — loses scale | same call, new parser preserves scale/E-notation; verify regex accepts what Java's ctor grammar accepts | S |
| 7 | `pkg/emit/emit.go:245` (AOT const reconstruction) | `MustBigDecimal(x.String())` | works iff `String()` round-trips scale — Java toString does (that's its design goal); add a round-trip test. REPL-vs-binary divergence guard lives here | S (test-heavy) |
| 8 | `pkg/lang/bigdecimal.go:99` `Hash` / `pkg/lang/equal.go:152` | hashes `big.Float.String()`; `Equals` is Cmp-based | hasheq via `StripTrailingZeros` (finding 1: `=` equal ⇒ hash equal); `Equals` stays Cmp-based for two BigDecimals | S |
| 9 | `pkg/eval/numeric_builtins.go:88-103` (`bigdec`) | string parse + `AsBigDecimal` | same shape; string errors get Java-style wording (optional, suite doesn't read messages) | S |
| 10 | `pkg/eval/numeric_builtins.go:53,79` + `pkg/lang/numbers.go:965,1002` (`bigint`/`biginteger`/`IntCast`/`LongCast` via `ToBigInteger`) | `big.Float.Int(nil)` | new `ToBigInt` (truncate toward zero, exact) — signature-compatible | S |
| 11 | `pkg/lang/numbers.go:128` (`Rationalize`), `pkg/eval/numeric_builtins.go:454` (`rationalize`) | `val.Text('f', -1)` string | `String()`/plain string of the new type (exact by construction) | S |
| 12 | `pkg/lang/numbers.go:601,690,790,828,844` (AsNumber/AsFloat64/Inc/IncP/IsNumber) + `pkg/lang/bigint.go:78` (`ToBigDecimal`) | type-switch arms + big.Float conversions | mechanical re-target (`AddInt`, `Float64`, scale-0 from BigInt) | S |
| 13 | **new**: `with-precision` + `*math-context*` (core.clj + eval) | absent (26-row baseline gap) | dynamic `*math-context*` (precision + RoundingMode, 8 modes); `+ - * /` on the decimal path consult it (`Round`/`DivideMC` exist in the prototype); `with-precision` macro binds it | M |
| 14 | `pkg/eval/format_render.go:47` + S14 deferred tail | `%f`/`%e` reject BigDecimal | third arg-kind branch rendering at the value's own scale (S14 VERDICT lines 127-133) | S |

Also touched by ripple, no code change expected: `pkg/reader/
syntaxquote.go:179` (self-evaluating const list), `pkg/eval/
predicate_builtins.go:160,173` (`decimal?` type check), `pkg/eval/
misc_builtins.go:128` (`instance?` name match), `conformance/tests/
numeric-bigint-bigdec.clj` (oracle-verified expectations must stay
green — they were frozen against real Clojure, so a faithful
representation keeps them passing; re-run to prove).

Sizing: 1 L + 2 M + 11 S. Items 1-8 are one coherent change (the
representation swap + its direct consumers) and should land as one
OpenSpec change; items 13-14 (`with-precision`, `format`) are a
follow-on change on top of it. Suite payoff: `bigdec.cljc` and
`with_precision.cljc` move from skipped/failing to passing, and the
`quot`/`mod`/`rem` decimal rows left open by ADR 0029 close.

## Known prototype limits (record in ADR 0032, none corpus-visible)

- `FromFloat64` uses Go's shortest-repr formatting; Java's
  `Double.toString` picks different **string** forms at some exponent
  switch-overs (e.g. `1.0E23` vs `1e+23`) but the parsed Dec value and
  scale are identical — only if Java's shortest digits ever differ
  (they don't; both are shortest-round-trip) would values diverge.
- `DivideToIntegral` with a negative preferred scale keeps scale 0
  (exact value, more digits than Java might use); not exercised by
  Clojure-reachable code paths found so far.
- `DivideMC` on exactly-representable quotients doesn't reproduce
  Java's trailing-zero/preferred-scale trimming (corpus rows are all
  non-terminating). Add oracle rows when wiring `with-precision`.
- Java caps scale at ±2^31−1 with overflow errors; the port clamps and
  panics (`clampScale`) — wording alignment only.

## Files

- `README.md` — question + exit criterion (written first).
- `probes.clj` / `probes_wp.clj` — the 159-row corpus.
- `run_probes.sh` — oracle + cljgo baseline + prototype harness.
- `proto/` — candidate-(a) prototype (`decimal.go`) + harness
  (`main.go`), own `go.mod` (fenced from the main module, like every
  spike).
- `out/` — frozen oracle outputs, cljgo baseline outputs,
  `proto_report.txt` (159/159), `diff` evidence for the table above.
