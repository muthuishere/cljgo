## ADDED Requirements

### Requirement: BigDecimal preserves decimal scale exactly
BigDecimal SHALL be represented as an unscaled arbitrary-precision
integer plus a 32-bit scale (value = unscaled × 10^−scale), Java
BigDecimal's exact model. Literals, string coercions, and conversions
SHALL preserve scale and value exactly — never mantissa-truncate.

#### Scenario: literal scale preserved (oracle 1.12.5)
- **WHEN** `1.10M`, `1.00M`, `0.000M` are read and printed
- **THEN** they print `1.10M`, `1.00M`, `0.000M` — trailing zeros kept

#### Scenario: long literals are exact (oracle 1.12.5)
- **WHEN** `123456789012345678901234567890.12M` is evaluated
- **THEN** the result is exactly `123456789012345678901234567890.12M`

#### Scenario: E-notation round-trips (oracle 1.12.5)
- **WHEN** `1E10M`, `1e2M`, `12345E-2M` are read
- **THEN** they print `1E+10M`, `1E+2M`, `123.45M` (javadoc toString)

### Requirement: arithmetic follows Java's scale rules
Add/subtract SHALL produce scale = max(sx, sy); multiply scale =
sx + sy; the results SHALL be exact decimal values (no binary
artifacts).

#### Scenario: decimal addition is exact (oracle 1.12.5)
- **WHEN** `(+ 1.1M 2.2M)` and `(+ 1.10M 2.2M)` are evaluated
- **THEN** the results are `3.3M` and `3.30M`

#### Scenario: multiplication sums scales (oracle 1.12.5)
- **WHEN** `(* 1.10M 2.0M)` is evaluated
- **THEN** the result is `2.200M`

### Requirement: division is exact-or-throw with preferred scales
`/` on the decimal path SHALL return the exact quotient at preferred
scale sx−sy (zero-padded when shorter), SHALL throw an
ArithmeticException when the exact quotient has a non-terminating
decimal expansion, and SHALL throw on a zero divisor — never Inf.
`quot`/`rem` SHALL follow divideToIntegralValue/remainder preferred
scales.

#### Scenario: exact and non-terminating division (oracle 1.12.5)
- **WHEN** `(/ 1M 4M)`, `(/ 1M 3M)`, `(/ 1M 3)`, `(/ 1M 0M)` are evaluated
- **THEN** `0.25M`; both `(/ 1M 3M)` and `(/ 1M 3)` throw
  non-terminating; `(/ 1M 0M)` throws divide by zero

#### Scenario: quot preferred scale (oracle 1.12.5)
- **WHEN** `(quot 10.0M 3)` and `(quot 10.0M 3.0M)` are evaluated
- **THEN** the results are `3.0M` and `3M`

### Requirement: equality is compareTo-based with normalized hasheq
`=` on two BigDecimals SHALL ignore scale (`(= 1.0M 1.00M)` true);
cross-category `=` SHALL be false even for equal values (`(= 1M 1)`
false, `==` true); hasheq SHALL normalize via stripTrailingZeros so
`=`-equal BigDecimals hash identically.

#### Scenario: scale-insensitive equality (oracle 1.12.5)
- **WHEN** `(= 1.0M 1.00M)`, `(= (hash 1.0M) (hash 1.00M))`,
  `(count (hash-set 1.0M 1.00M))` are evaluated
- **THEN** true, true, 1

#### Scenario: cross-category (oracle 1.12.5)
- **WHEN** `(= 1M 1)`, `(= 1M 1N)`, `(= 0.5M 1/2)`, `(== 1M 1)` are evaluated
- **THEN** false, false, false, true

### Requirement: BigDecimal cannot hold Inf/NaN
Coercing a non-finite double to BigDecimal SHALL throw
`Infinite or NaN`; finite doubles convert via valueOf semantics
(shortest decimal representation).

#### Scenario: Inf/NaN throw (oracle 1.12.5)
- **WHEN** `(bigdec ##Inf)` or `(bigdec ##NaN)` is evaluated
- **THEN** each throws

#### Scenario: valueOf semantics (oracle 1.12.5)
- **WHEN** `(bigdec 0.1)`, `(bigdec -0.0)`, `(bigdec 1.5e300)` are evaluated
- **THEN** `0.1M`, `0.0M`, `1.5E+300M`

### Requirement: printing follows the javadoc toString algorithm
`pr`/`pr-str` SHALL print Java's toString (plain iff scale ≥ 0 and
adjusted exponent ≥ −6, else scientific with signed exponent) plus the
`M` suffix; `str` SHALL print the same without the suffix. Interpreted
and AOT-compiled output SHALL be byte-identical (the emitter
reconstructs constants from the same toString).

#### Scenario: plain-vs-E boundary (oracle 1.12.5)
- **WHEN** `(str 0.000001M)` and `(str 0.0000001M)` are evaluated
- **THEN** `"0.000001"` and `"1E-7"`
