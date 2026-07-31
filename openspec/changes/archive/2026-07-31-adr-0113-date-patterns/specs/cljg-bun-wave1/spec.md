## ADDED Requirements

### Requirement: cljg.date formats and parses java.time patterns

`cljg.date` MUST provide `format-pattern` and `parse-pattern` accepting a
**java.time** pattern string, so a portable `.cljc` library can use one pattern
vocabulary across hosts with its `:clj` branch calling
`DateTimeFormatter/ofPattern` untranslated.

A compiled pattern MUST be represented as a sequence of independent literal
and field operations. It MUST NOT be represented as a Go reference-time layout
string, because Go's `time.Format` substitutes a text token conditionally on
the literal that follows it, which makes an adjacent literal able to silently
suppress a field.

Output MUST be byte-identical to `java.time` for every pattern accepted, under
`Locale.ENGLISH`.

#### Scenario: a text token adjacent to a literal still formats

- GIVEN the pattern `EEE'at' Z`
- WHEN an instant is formatted with it
- THEN the output matches java.time's, with the weekday substituted
- AND the weekday is NOT emitted as the literal text of the pattern token

#### Scenario: the fraction needs no separator

- GIVEN the patterns `HH:mm:ss.SSS`, `HH:mm:ss,SSS` and `SSS`
- WHEN an instant is formatted with each
- THEN each output matches java.time's, including the bare fraction

### Requirement: an unrepresentable pattern is refused by name, at compile time

Compiling a pattern MUST fail with a registered diagnostic naming the offending
token when the token cannot be represented exactly — era, quarter, day-of-year,
week-of-year, week-of-month, day-of-week-in-month, clock-hour variants,
week-based year, proleptic year, zone name, zone id — or when a token's run
length is invalid.

The failure MUST occur when the pattern is compiled, never when an instant is
formatted, and a token MUST NEVER be silently dropped or approximated.
A pattern `java.time` itself rejects MUST NOT be accepted.

#### Scenario: an unsupported token names itself

- GIVEN the pattern `QQ yyyy`
- WHEN it is compiled
- THEN compilation fails with a registered code and the message names `QQ`
- AND no instant is formatted

#### Scenario: cljgo does not accept what the JVM rejects

- GIVEN a pattern `java.time` rejects, such as `EEEEEEE` or `aa`
- WHEN it is compiled
- THEN compilation fails

### Requirement: pattern compilation is memoised, never per call

Formatting with the same pattern repeatedly MUST NOT recompile it. Per-call
compilation costs 6× the time and 60× the allocation of the compiled path,
which is the term that matters on a server path.

#### Scenario: repeated formatting does not recompile

- GIVEN a pattern formatted many times
- WHEN the same pattern string is used
- THEN the compiled form is reused
- AND concurrent use from many goroutines is race-free
