# Apply ADR 0032 — BigDecimal is an unscaled big.Int + int32 scale

## Why

ADR 0032 (docs/adr/0032-bigdecimal-unscaled-int-plus-scale.md, accepted)
mandates replacing the `big.Float` inside `pkg/lang/bigdecimal.go` with
Java BigDecimal's exact model: unscaled `*big.Int` + `int32` scale. The
current representation loses scale (`1.10M` prints `1.1M`), represents
Inf/NaN (Java's cannot), returns binary artifacts from decimal arithmetic
(`(+ 1.1M 2.2M)` → `3.3000000000000000002M`), never throws on
non-terminating division, and — decisively — SILENTLY CORRUPTS long
literals via mantissa truncation. Spike S16
(spikes/s16-bigdecimal-scaled/VERDICT.md) proved the replacement: a ~450
line stdlib-only prototype matching real Clojure 1.12.5 on 159/159
oracle corpus rows.

## What Changes

Items 1–8 of the S16 migration inventory — one coherent representation
swap plus its direct consumers:

- **1 — the type** (`pkg/lang/bigdecimal.go`, whole file): port the S16
  prototype (`proto/decimal.go`): Java's BigDecimal(String) parse
  grammar, scale-0 int/BigInt constructors, valueOf(double) semantics
  (shortest decimal string; Inf/NaN throw), add/sub scale = max, mul
  scale = sum, exact divide with preferred scale `sx−sy`
  (non-terminating/zero → ArithmeticException), divideToIntegralValue +
  remainder with Java's preferred-scale rules, the javadoc toString
  plain-vs-E algorithm, stripTrailingZeros. Exported names
  (`NewBigDecimal`, `MustBigDecimal`, `NewBigDecimalFrom*`,
  `ToBigInteger`, `Cmp`, …) are kept so callers mostly don't move;
  the big.Float-shaped members (`NewBigDecimalFromBigFloat`,
  `ToBigFloat`) are dropped with their callers retargeted.
- **2 — Ops matrix rows** (`pkg/lang/numberops.go` bigDecimalOps):
  re-target to the new API; `Divide` throws non-terminating/zero;
  `Quotient`/`Remainder` use the preferred-scale methods; `Equiv` stays
  Cmp-based (oracle: `(= 1.0M 1.00M)` is TRUE).
- **3 — `AsBigDecimal`** (`pkg/lang/numberops.go`): ints → scale-0
  unscaled directly (closes the >2^53 precision loss); float64 via
  valueOf semantics, throwing `Infinite or NaN` — closes
  `(bigdec ##Inf)`.
- **4 — Combine matrix**: verify-only; `(+ 1.0 1.0M)` must stay
  float64.
- **5 — printer** (`pkg/lang/strconv.go`): pr prints Java `toString()` +
  `"M"` (kills the `1.10M`→`1.1M` bug); `str` prints `toString()`
  without suffix.
- **6 — reader M literal** (`pkg/reader/number.go`): same
  `NewBigDecimal` call; the new parser preserves scale/E-notation and
  never truncates long literals.
- **7 — AOT const reconstruction** (`pkg/emit/emit.go`):
  `MustBigDecimal(x.String())` round-trips exactly under Java toString;
  conformance dual harness proves REPL-vs-binary byte-identity.
- **8 — equality/hasheq** (`pkg/lang/bigdecimal.go` Hash,
  `pkg/lang/equal.go`): hasheq via stripTrailingZeros so `=`-equal
  BigDecimals hash alike; `Equals` stays Cmp-based for two BigDecimals
  and false cross-category.

Ripple re-targets (mechanical, same inventory): `AsInt64`/`AsFloat64`/
`Rationalize`/`IntCast`/`LongCast` arms in `pkg/lang/numbers.go`,
`BigInt.ToBigDecimal` in `pkg/lang/bigint.go`, `rationalize` in
`pkg/eval/numeric_builtins.go`.

The S16 corpus lands as grouped `conformance/tests/bigdec-*.clj` files
with frozen, oracle-cited expectations (dual-harness).

## Non-goals

- **Items 13–14** (`with-precision` + `*math-context*`, format
  `%f`-on-BigDecimal) — the follow-on change per ADR 0032; not here
  unless items 1–8 land cleanly with time to spare.
- Exception-message wording parity beyond what the new code paths
  produce naturally (the suite's `p/thrown?` doesn't read messages).

## Capabilities

### New Capabilities
- `bigdecimal-scaled`: Java-faithful scaled-decimal BigDecimal — literal
  scale preservation, arithmetic scale rules, exact-or-throw division,
  compareTo-based equality with normalized hasheq, javadoc toString.

### Modified Capabilities
(none — openspec/specs/ has no existing capability this touches)

## Impact

- ADRs relied on: **0032** (this change implements it), 0029 (completes
  its deferred cluster C), 0022 (suite compliance), 0002 (dual-mode).
- Code: `pkg/lang/bigdecimal.go` (rewritten — logged in PROVENANCE.md),
  `pkg/lang/{numberops,numbers,strconv,bigint}.go`,
  `pkg/reader/number.go`, `pkg/eval/numeric_builtins.go`,
  `conformance/tests/`.
- Evidence: suite `bigdec.cljc` moves fail → pass; the corpus-derived
  conformance files freeze the oracle rows.
