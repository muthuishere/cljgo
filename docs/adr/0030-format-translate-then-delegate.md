# ADR 0030 — `format` is Java's grammar, implemented translate-then-delegate
Date: 2026-07-16 · Status: accepted · Evidence: spike S14 (spikes/s14-format-grammar/VERDICT.md)

## Context

Clojure's `format` is Java String.format grammar; Go's fmt differs (%b,
grouping, parens-negative, $-indexing, %n). S14 prototyped both a direct
interpreter (236 lines) and translate-then-delegate (148 lines) against an
80-probe oracle corpus: both hit 80/80.

## Decision

Ship **translate-then-delegate**: parse the Java format string, delegate
the compatible core (d x o c s f e) to fmt.Sprintf, hand-implement %b, %g,
the `,`/`(` flags, $/< indexing, %n. Same measured compatibility, ~90
fewer lines. Discipline: a strict per-verb flag allow-list — Go's fmt
fails silently on bad combos, so nothing unvetted reaches it. Full
conversion set from day one (no MVP subset — corpus-cheap).
`(format "%q")` on an unknown conversion throws, matching Java's
UnknownFormatConversionException.

## Consequences

`format`/`printf` become faithful for real-world use. Recorded tail:
half-to-even vs Java HALF_UP rounding on %f/%e/%g ties (unmeasured),
javaDoubleToString extremes unfuzzed, BigDecimal/with-precision deferred
with cluster C of ADR 0029.
