## ADDED Requirements

### Requirement: float64 quot/rem guard non-finite operands like the JVM
`quot`, `rem`, and `mod` on double operands SHALL mirror JVM
`clojure.lang.Numbers.quotient/remainder(double,double)`: a 0.0 divisor
SHALL throw `Divide by zero`; when the raw quotient x/y falls outside int64
range and is Inf or NaN the operation SHALL throw `Infinite or NaN`; when it
falls outside int64 range but is finite the result SHALL be a double
computed via exact big-integer truncation; otherwise the result SHALL be the
double of the truncated quotient (quot) or `x - trunc(x/y)*y` (rem).

#### Scenario: Inf/NaN operands throw (oracle 1.12.5)
- **WHEN** `(quot ##Inf 1)`, `(rem ##NaN 1)`, `(mod 5 ##NaN)`, or `(rem 10.0 0)` is evaluated
- **THEN** each throws — `Infinite or NaN` for the non-finite quotients, `Divide by zero` for the 0.0 divisor

#### Scenario: finite edges keep JVM values (oracle 1.12.5)
- **WHEN** `(quot 1 ##Inf)`, `(rem 1 ##-Inf)`, `(mod 1 ##Inf)`, `(quot 1e300 1.0)` are evaluated
- **THEN** they return `0.0`, `##NaN`, `##NaN`, and `1.0E300` respectively

### Requirement: AsBigInt of a double converts exactly like the JVM
`bigint`/`biginteger` of a double (and the internal float→BigInt
conversion) SHALL follow `BigDecimal.valueOf(double)` semantics — the
shortest round-trip decimal representation truncated toward zero — never a
saturating int64 cast; Inf/NaN SHALL throw `Infinite or NaN`.

#### Scenario: Double/MAX_VALUE (oracle 1.12.5)
- **WHEN** `(bigint 1.7976931348623157e+308)` is evaluated
- **THEN** the result is `17976931348623157` followed by 292 zeros, with the N suffix

#### Scenario: decimal, not binary, expansion (oracle 1.12.5)
- **WHEN** `(bigint 4.611686018427388E18)` is evaluated
- **THEN** the result is `4611686018427388000N` (shortest-decimal truncation), not the exact binary value `4611686018427387904N`

### Requirement: even? and odd? accept only integers
`even?` and `odd?` SHALL throw an IllegalArgument error with message
`Argument must be an integer: <str of n>` for any non-integer argument
(doubles, ratios, bigdecs, ##Inf/##NaN, nil), exactly as JVM clojure.core.

#### Scenario: non-integer throws (oracle 1.12.5)
- **WHEN** `(even? 1.5)` or `(odd? 1/2)` is evaluated
- **THEN** it throws `Argument must be an integer: 1.5` / `Argument must be an integer: 1/2`

### Requirement: abs is a clojure.core builtin over the whole tower
`abs` SHALL resolve in both modes and dispatch through the numeric tower:
`(abs -1)` is `1`, `(abs -1/5)` is `1/5`, `(abs -123N)` is `123N`,
`(abs -123.456M)` is `123.456M`, `(abs -0.0)` is `0.0`, `(abs ##-Inf)` is
`##Inf`, `(abs ##NaN)` is NaN, and `(abs Long/MIN_VALUE)` is Long/MIN_VALUE
(JVM 2's-complement identity).

#### Scenario: abs resolves and matches the oracle (oracle 1.12.5)
- **WHEN** `(abs -9223372036854775808)` is evaluated
- **THEN** the result is `-9223372036854775808` and no resolution error occurs

### Requirement: int bounds against Java's 32-bit int
`(int x)` SHALL range-check against int32, not the platform int: integral
inputs outside [-2147483648, 2147483647] SHALL throw `integer overflow`;
double inputs outside that range SHALL throw `Value out of range for int:
<Double.toString of x>`; a BigInt beyond int64 SHALL throw `Value out of
range for long: <n>` (JVM longCast fails first).

#### Scenario: 64-bit values no longer pass (oracle 1.12.5)
- **WHEN** `(int 2147483648)` or `(int (bigint 3000000000))` is evaluated
- **THEN** each throws `integer overflow`

#### Scenario: double just outside the range (oracle 1.12.5)
- **WHEN** `(int 2147483647.000001)` is evaluated
- **THEN** it throws `Value out of range for int: 2.147483647000001E9`
