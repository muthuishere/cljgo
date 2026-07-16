# S14 — Java format grammar on a Go host

## Question

Clojure's `format` is literally Java's `String.format` / `java.util.Formatter`
grammar: `%s %d %f %e %g %x %o %c %b %n %%`, `%1$s`-style argument indexing
(plus `%<` relative indexing), and flags `- + ' ' 0 , ( #` combined with
width/precision. Go's `fmt` package overlaps syntactically (`%s %d %f %x %o`)
but diverges in ways that are NOT cosmetic:

- `%b` means **binary** in Go, **boolean** in Java/Clojure.
- Go has no `,` grouping flag, no `(` negative-parens flag, no `$` argument
  index (Go's analogous feature is `%[n]d`), no `%n` newline conversion.
- Go's `%x`/`%o` on negative ints print a `-` sign; Java's print the
  two's-complement bit pattern (large positive value).
- nil/`null` handling differs (`%s` of nil argument → literal string `"null"`
  in Java; Go's `%s` of `nil` depends on the static type of the arg).

`format` is one of the two remaining blockers on the conformance suite
(`conformance/tests`, and `clojure-test-suite/test/clojure/core_test/format.cljc`
upstream) and it is *everywhere* in real Clojure code (log messages, CLI
output, string templating) — so partial/incorrect support poisons adoption
more than an outright `Unbound`.

**The spike question: do we port the FULL Java format grammar to Go, or ship
a translated subset — and if a subset, exactly which conversions/flags are
in vs out for the first cut (feeds ADR 0030)?**

## Exit criterion

Exit = both of:

1. **A corpus-verified compatibility report** — every probe run against real
   JVM Clojure 1.12.5 (the oracle) AND against two candidate Go
   implementations, with an exact-match rate per candidate and a list of
   every divergence and why.
2. **A working prototype of the recommended approach** — not full production
   code, but enough of a real implementation (not a mock) that the corpus
   numbers in (1) are measured against actual behavior, not intent.

## Method

1. `corpus.go` — ~90 probes: the 2 real assertions from the jank-derived
   suite (`clojure-test-suite/test/clojure/core_test/format.cljc`) plus a
   hand-built systematic sweep: every conversion, every flag on every
   conversion it can legally combine with, width/precision combinations,
   positional (`%1$s`) and relative (`%<`) indexing, `%n`/`%%`, nil
   arguments, BigInt/Ratio/Double arguments, `%b` on non-boolean values,
   and a few "should throw" cases (unknown conversion, missing argument).
2. `gen_oracle.go` — emits one big `.clj` script from the same corpus
   (single source of truth: each `Probe` carries both its Go args and the
   literal Clojure source for the same args) and shells out to
   `clojure -M -e` ONCE for the whole corpus (one JVM boot, not 90) to
   avoid ~1–2s/probe startup tax. Captures exact stdout or the exception's
   simple class name per probe → `oracle.json`.
3. Two candidate implementations, run over the same corpus:
   - `translate.go` — **translate-then-delegate**: parse the Java-style
     format string into a directive list, rewrite directives whose Go
     `fmt` verb means the same thing straight into a Go format string
     (`%d`, `%x`, `%o`, `%c`, plain `%s`/width/precision), and
     hand-implement only the genuinely divergent bits (`%b` boolean,
     `,`/`(` flags, `$`/`<` indexing, `%n`, two's-complement `%x`/`%o`).
   - `direct.go` — **direct interpreter**: never touches Go's `fmt`
     verbs at all; parses the full Java directive grammar and renders
     every conversion by hand (own padding/sign/grouping/hex/exponent
     code).
4. `run_test.go` compares both candidates' output against `oracle.json`
   and prints an exact-match rate + a divergence table.
5. `VERDICT.md` — the recommendation for ADR 0030: which approach, what
   subset is defensible to ship first, and what an unsupported conversion
   should do (Java throws `UnknownFormatConversionException`).

## Scope note

`cl-format` (pretty-printing, `~a ~s ~d`, column alignment) is NOT in scope —
that rides on `clojure.pprint`, a different machinery entirely. `printf` is
in scope (it's `(print (format ...))` plus flush — no separate grammar).
`with-precision`/BigDecimal rounding-mode formatting is noted as a followup
risk but not exhaustively probed here.
