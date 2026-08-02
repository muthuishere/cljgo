## ADDED Requirements

### Requirement: format renders Java Formatter grammar
The system SHALL provide `(format fmt & args)` implementing the
`java.util.Formatter` grammar subset (conversions `s S d x X o c C b B f e E
g G n %`; flags `- + (space) 0 , ( #`; width; precision; argument indexing
`%N$`/`%<`) via translate-then-delegate (ADR 0030): the fmt.Sprintf-
compatible core (`d x o c s f e` without `,`/`(`) delegates to Go's
`fmt.Sprintf` through a strict per-verb flag allow-list; `%b`, `%g`, the
`,`/`(` flags, `%n`, and argument-index resolution are hand-rendered, since
Go's `fmt` has no equivalent. Behavior SHALL be identical in interpreted and
AOT-compiled mode.

#### Scenario: passthrough with no directives
- **WHEN** `(format "test")` is evaluated
- **THEN** the result is `"test"`

#### Scenario: basic conversions
- **WHEN** `(format "%s is %d years old (%.1f%%)" "Alice" 30 12.345)` is evaluated
- **THEN** the result is `"Alice is 30 years old (12.3%)"`

#### Scenario: comma grouping and parens have no Go fmt equivalent but work
- **WHEN** `(format "%,d" 1234567)` is evaluated
- **THEN** the result is `"1,234,567"`

#### Scenario: argument indexing and relative reuse
- **WHEN** `(format "%2$s %1$s %<s" "a" "b")` is evaluated
- **THEN** the result is `"b a b"`

### Requirement: format rejects unrecognized conversions and type mismatches
The system SHALL throw when a conversion letter has no defined meaning
(`UnknownFormatConversionException`-shaped message) or when an argument's
runtime type does not satisfy the conversion's type requirement
(`IllegalFormatConversionException`-shaped message) — `%d`/`%x`/`%o` require
a plain `long` (rejecting BigInt/Ratio/double, matching real Clojure), `%c`
requires a genuine char (rejecting `long`), `%f`/`%e`/`%g` require a
`double`. No flag/verb combination unvetted by the per-verb allow-list may
reach `fmt.Sprintf` unfiltered.

#### Scenario: unknown conversion throws
- **WHEN** `(format "%q" 1)` is evaluated
- **THEN** an error is thrown whose message names `UnknownFormatConversionException`

#### Scenario: type mismatch throws
- **WHEN** `(format "%d" 3.14)` is evaluated
- **THEN** an error is thrown whose message names `IllegalFormatConversionException`

#### Scenario: BigInt/Ratio rejected by %d/%f like real Clojure
- **WHEN** `(format "%d" 100000000000000000000N)` or `(format "%f" 1/3)` is evaluated
- **THEN** an error is thrown whose message names `IllegalFormatConversionException`

### Requirement: printf writes formatted output without a new write surface
The system SHALL provide `(printf fmt & args)` equivalent to `(print (format
fmt args...))`, writing through the same output path `println` already uses
(`eval.Out`), so it composes with whatever component owns dynamic `*out*` /
`with-out-str` without introducing a second write surface.

#### Scenario: printf writes the formatted string
- **WHEN** `(printf "%s=%d" "x" 1)` is evaluated
- **THEN** the text `"x=1"` is written to the current output stream and the form returns `nil`
