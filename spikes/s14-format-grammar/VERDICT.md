# S14 VERDICT — Java format grammar on a Go host

## Result

**80/80 probes exact-match against real JVM Clojure 1.12.5 for BOTH
candidates.** Corpus: the 2 real assertions from
`clojure-test-suite/test/clojure/core_test/format.cljc` (upstream jank-derived
suite) plus 78 hand-built probes sweeping conversions, flags, width/precision,
argument indexing, nil, BigInt/Ratio, `%b` truthiness, and 8 "should throw"
cases. Oracle: one `clojure -M -e` invocation covering the whole corpus
(~0.4s), not one JVM boot per probe.

Both `direct.go` (236 lines, never touches Go's `fmt` verb formatter — all
strconv + hand padding) and `translate.go` (148 lines, delegates the
compatible core to `fmt.Sprintf` and hand-writes only the genuinely
divergent bits) reached 100% once two real bugs were found and fixed:

1. `%S` (uppercase string) — translate's first pass forgot to uppercase the
   *result* of the delegated `fmt.Sprintf`, only direct.go did (a real,
   easy-to-miss bug specific to the translate approach: Go's `%s` has no
   case-folding flag, so uppercasing is always a manual post-step regardless
   of which candidate you pick).
2. `%s` of a whole-number `double` (`3.0`) — Go's default float text
   (`strconv`/`%v`, shortest round-trip, no forced `.0`) disagrees with
   `java.lang.Double.toString` (always includes a decimal point; different
   sci-notation threshold: 1e-3/1e7 vs Go's format-dependent switch). This
   is **shared infrastructure**, not a translate-vs-direct question — `%s`
   works by calling `.toString()` on the raw argument in Java, so BOTH
   candidates route through the same `toDisplayString`/`javaDoubleToString`
   helper. Fixed once in `common_render.go`, fixed for both.

## Recommendation: translate-then-delegate (A), NOT a full direct port

**Ship candidate A.** Reasoning, from what actually varied between the two
120-line implementations:

- **The value-rendering split falls into three buckets, and the boundary
  is sharp, not fuzzy:**
  - **Shared, regardless of approach** — argument-index resolution
    (`$`/`<`), the flag-validation errors (duplicate flags, `-0` conflict),
    `%n`, `%%`, and the null-before-type-check special case. These live in
    `dispatch.go`/`spec.go`, written ONCE, used by both candidates. This is
    ~200 of the corpus's 1161 total lines and is not part of the "which
    approach" decision at all.
  - **Delegates cleanly to Go's `fmt`** — `%d`, `%x`/`%o` (cast to `uint64`
    first — this "for free" reproduces Java's two's-complement bit pattern
    on negatives, which is the one behavior Go's own `%x` on a *signed* int
    gets wrong relative to Java), `%c` (with a hand type-check: Java's `%c`
    requires `Byte`/`Short`/`int`/`Character`, and — confirmed against the
    oracle — a plain Clojure `Long` does **not** qualify and throws
    `IllegalFormatConversionException`; only a genuine char literal works),
    `%s`/`%f`/`%e` with the standard flags. Comma-grouping and paren-negative
    (`,`/`(`) have zero Go equivalent, so even in the "delegates cleanly"
    bucket, translate still hand-renders the digits then re-pads — but that
    hand code is identical in spirit whether or not you started from an
    `fmt.Sprintf` call, so the LOC savings are real and not illusory.
  - **No Go equivalent, hand-write regardless of approach** — `%b`
    (Go's `%b` means binary; Java's is a truthiness check: `nil`→false,
    non-`Boolean`→true unconditionally, matching Boolean value otherwise —
    confirmed via `b-truthy-zero`/`b-truthy-string` oracle probes both
    printing `"true"`) and `%g` (Java's algorithm — round to `precision`
    significant digits, then choose fixed vs. scientific by comparing the
    POST-ROUNDING exponent against `[-4, precision)` — has no Go analogue;
    Go's `%g` picks shortest-round-trip representation, a different goal
    entirely). `direct.go` and `translate.go` call the exact same
    `directB`/`directG` functions for these — there is no delegation path.

- **Where the delegation actually pays off** (`%d`/`%x`/`%o`/`%c`/`%s`/`%f`/
  `%e` without `,`/`(`): direct.go needs ~90 more lines than translate.go
  to hand-roll sign/pad/case logic that Go's `fmt.Sprintf` already gets
  right — and every one of those lines is a place a future edge case can
  silently diverge from Java that translate simply doesn't have to get
  right in the first place (Go's own team has already fuzzed `%d`/`%f`
  width/precision/sign combinatorics).
- **The cost of translate is discipline, not code**: `buildVerb`'s
  `goFlags` allow-list (`spikes/s14-format-grammar/translate.go`) exists
  *only* to keep an invalid flag from ever reaching `fmt.Sprintf`, which
  would otherwise emit `%!d(BADVERB)`-style noise into real output instead
  of failing loudly. That allow-list has to be reviewed any time a new
  flag/conversion combination is added — a direct interpreter has no
  equivalent silent-failure mode because it never calls the Go formatter at
  all. Call this out explicitly in the eventual implementation's tests: a
  translate-then-delegate `format` needs a "never let an unfiltered flag
  reach `fmt.Sprintf`" invariant test, not just conformance probes.

## What to ship first (subset for the initial cut)

All of the following are corpus-verified at 100% and cheap either way —
ship the full set, not a smaller "MVP":

- Conversions: `s S d x X o c C b B f e E g G n %`.
- Flags: `- + (space) 0 , ( #`, including the illegal-combo errors
  (`DuplicateFormatFlagsException`, `IllegalFormatFlagsException` for `-0`).
- Width, precision, argument indexing (`%1$s`, `%<`).
- `IllegalFormatConversionException`-class type checks: `%d`/`%x`/`%o`/`%c`
  reject anything that isn't a plain Go `int64` (cljgo's `long`) —
  **explicitly including BigInt and Ratio, which real Clojure ALSO
  rejects** (`d-bigint-huge`, `d-ratio-throws` probes) — so cljgo does not
  need arbitrary-precision `%d` support to match real Clojure; matching
  real Clojure here means rejecting them, which is simpler than accepting
  them.
- `%f`/`%e`/`%g` reject non-`float64` args (ints/Ratio/BigInt) the same way.

**`%q` and any letter outside the above → `UnknownFormatConversionException`**
— matches Java exactly (confirmed: `unknown-conversion` probe) and should
be cljgo's behavior for `(format "%unsupported" ...)` verbatim: throw, don't
silently pass the directive through or print garbage. This also covers the
date/time family (`%t`/`%T`) — genuinely out of scope (see README), and
should throw the same way rather than being half-supported.

## Known-incomplete tail (flag for the eventual `format` spec/ADR 0030)

- **Rounding mode**: `%f`/`%e`/`%g` here delegate to `strconv.FormatFloat`,
  which rounds half-to-even; `java.util.Formatter` specifies `HALF_UP`.
  These diverge only at exact tie cases (e.g. rounding `X.125` to 2 decimal
  places) — none of the 80 probes happened to hit a tie, so this is a real,
  UNMEASURED gap, not a false confidence number. A production `format` needs
  either a decimal (`math/big.Float`/manual) rounding step for ties, or an
  explicit ADR 0030 call that half-to-even is an accepted, documented
  divergence (it will surface as a flaky-looking off-by-one-ULP bug in user
  code that formats currency, which is exactly where people notice).
- **`javaDoubleToString`** (`%s` of a bare `double`) approximates
  `Double.toString`'s scientific-notation threshold and exponent spelling;
  not fuzzed against the full `double` range (subnormals, `Double.MIN_VALUE`
  neighborhood). Low real-world risk (rare to `%s` a raw double instead of
  `%f`/`%g` it) but not proven at the tails.
- **BigDecimal / `with-precision`**: not probed at all. `format`'s `%f`/`%e`
  in real Java also accepts `BigDecimal` and honors its own scale.
  `pkg/lang/bigdecimal.go` already has a `BigDecimal` type (wrapping
  `*big.Float`, itself binary not decimal — a known TODO in that file), but
  no `with-precision` form is wired in `core/` yet. When it lands, `%f`/`%e`
  need a third arg-kind branch (`*lang.BigDecimal`) that renders at the
  value's OWN scale rather than the conversion's default/given precision —
  a real interaction this spike did not exercise, flagged for whoever wires
  `with-precision`, not a blocker for this format grammar decision.
- **Locale**: Java's Formatter is locale-sensitive (grouping separator,
  decimal point); this spike and the recommended subset are US/default-locale
  only, matching cljgo's existing no-locale-support posture elsewhere.

## Sibling functions

- **`printf`** is in scope and free: `(printf fmt args...)` is `(print
  (format fmt args...))` plus a flush — no separate grammar, no extra work
  once `format` exists.
- **`cl-format`** (pprint's `~a ~s ~d` grammar) is explicitly NOT this
  machinery — different directive syntax, different engine (`clojure.pprint`),
  separate future spike if ever needed.
