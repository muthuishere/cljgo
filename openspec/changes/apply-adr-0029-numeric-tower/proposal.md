## Why

ADR 0029 (docs/adr/0029-numeric-tower-fidelity.md, accepted) mandates fixing
the five guard-clause-sized numeric-tower divergence clusters that spike S13
(spikes/s13-numeric-divergences/VERDICT.md) isolated against real Clojure
1.12.5: 99/276 probes diverge, and clusters A/B/D/E/F account for the suite's
entire quot/mod/rem/even?/odd?/abs/int fail buckets. The design work was the
spike — every fix below carries an exact pkg/lang citation and a frozen
oracle output.

## What Changes

- **A — float64 quot/rem guard ##Inf/##NaN** (`pkg/lang/numberops.go`):
  `float64Ops.Quotient`/`Remainder` mirror JVM `Numbers.quotient/remainder
  (double,double)`: divisor 0.0 throws `Divide by zero`; a quotient outside
  int64 range that is Inf/NaN throws `Infinite or NaN` (JVM's
  `new BigDecimal(double)` ctor); finite huge quotients return a double via
  the big-integer truncation. `mod` (core.clj, built on `rem`) inherits the
  fix.
- **B — `AsBigInt(float64)` exact conversion** (`pkg/lang/numberops.go`):
  replaces the saturating `int64(x)` with the JVM path —
  `BigDecimal.valueOf(double)` semantics, i.e. the shortest-round-trip
  decimal representation truncated toward zero; Inf/NaN throw
  `Infinite or NaN`. Fixes `bigint`/`biginteger` of large doubles and the
  cluster-A fallback.
- **D — `even?`/`odd?` integer guard** (`core/core.clj`): non-integer
  arguments throw IllegalArgument `Argument must be an integer: <n>` exactly
  as JVM clojure.core (via the existing `-illegal-argument` builtin).
- **E — `abs` wired as a builtin** (`pkg/eval`): registers the existing
  `lang.Abs` tower dispatcher (per-type `Ops.Abs` already vendored) as
  clojure.core/abs. `(abs Long/MIN_VALUE)` stays MIN (JVM 2's-complement),
  `(abs -0.0)` is `0.0`, `(abs ##-Inf)` is `##Inf`, `(abs ##NaN)` is NaN.
- **F — `IntCast` bounds against Java's 32-bit int** (`pkg/lang/numbers.go`):
  integral inputs outside int32 throw `integer overflow` (JVM 1.12
  Math.toIntExact path, oracle-verified for boxed and literal longs, BigInt,
  BigDecimal, Ratio); doubles outside int32 throw
  `Value out of range for int: <Double.toString>`; BigInt outside int64
  throws `Value out of range for long: <n>` (longCast fails first).
- Every fixed behavior gets a `conformance/tests/*.clj` file with frozen,
  oracle-cited `;; expect:` output (dual-harness).
- `pkg/lang` surgery is logged in `pkg/lang/PROVENANCE.md`.

## Non-goals

- **Cluster C (BigDecimal representation)** — explicitly deferred by ADR 0029
  to its own future ADR; `bigdecimal.go` is untouched.
- **Cluster G (exception-message wording)** — out of scope per ADR 0029;
  only messages on the code paths clusters A/B/D/F rewrite anyway are
  aligned with the oracle (they come for free with the guards).
- No changes to int64Ops/bigIntOps/ratioOps/bigDecimalOps
  Quotient/Remainder — only the float64 pair diverged.

## Capabilities

### New Capabilities
- `numeric-tower-fidelity`: JVM-faithful guards and conversions for the
  float64 quot/rem path, AsBigInt(float64), even?/odd?, abs, and int.

### Modified Capabilities
(none — openspec/specs/ has no existing capability this touches)

## Impact

- ADRs relied on: **0029** (this change implements it), 0022 (suite
  compliance), 0002 (dual-mode — all fixes live in pkg/lang / pkg/eval /
  core.clj, shared by both modes by construction).
- Code: `pkg/lang/numberops.go`, `pkg/lang/numbers.go` (vendored — logged in
  PROVENANCE.md), `pkg/eval/numeric_builtins.go`, `core/core.clj`,
  `conformance/tests/`.
- Evidence: spike S13 probes re-run after the fix must show the divergence
  count drop (89/266 baseline; clusters C and G rows remain by design).
