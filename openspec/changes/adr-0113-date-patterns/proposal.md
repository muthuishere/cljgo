# adr-0113-date-patterns — cljg.date gets java.time patterns, compiled to ops

## Why

`cljg.date/format` takes a **Go reference-time layout** (`"2006-01-02"`). Its
own docstring admits there is no JVM equivalent and warns that a java.time
pattern "would silently mis-format". That is a Go implementation detail
leaking through a `cljg.*` surface — the exact thing `cljg.*` exists to
prevent — and it cost a real consumer a real capability: koine **dropped
pattern formatting entirely** rather than leak Go layouts into portable
`.cljc`. Asked to name an API whose shape pushes work over the host boundary,
this is the example they gave.

Spike **s71** (`spikes/s71-date-patterns/RESULTS.md`) built the replacement and
stress-tested it differentially against the JVM oracle — 4,000 generated
patterns formatted on both hosts and diffed. It killed the obvious design.
Translating a java.time pattern into a **Go layout string** diverged on 1,252
of 4,000; three token fixes took it to 108, where it stops, because Go's
`Format` decides whether to substitute a text token based on the literal that
FOLLOWS it (`Format("Mon-at")` is `"Fri-at"`, `Format("Monat")` is `"Monat"`).
Compiling to an **op list** instead — no second string language, no re-parse —
scores **0 divergences** and refuses **15** patterns instead of 1,128, at
identical allocation.

## What Changes

- New `cljg.date/format-pattern` and `parse-pattern` taking a java.time
  pattern.
- Patterns compile to an **op list**, not to a Go layout string.
- The compile is **memoised**; it is never done per call (6× slower, 60× the
  allocation).
- Unrepresentable tokens are a **coded refusal naming the token**, at compile
  time.
- Text tokens are English; the docstring says `Locale.ENGLISH` explicitly, and
  says **not** `Locale.ROOT`.
- The existing Go-layout `format`/`parse` keep working, unchanged.

## Impact

- **Affected specs:** `cljg-bun-wave1`
- **Affected code:** `core/cljg/date.cljg`, `pkg/corelib` (host fns),
  `pkg/diag/registry.go`, `docs/diagnostics/`
- **Not affected:** `clojure.core` (which has no date formatting, so there is
  no precedence conflict), the existing `format`/`parse`/`format-iso` surface.

## Non-goals

- Locale-aware formatting beyond English. Go's stdlib has no locale data; the
  honest answer is English or a refusal, not a bundled CLDR.
- Comptime folding of literal patterns: measured at 7.6% over the memoised
  path at identical allocation, for a second code path through the analyzer.
  Declined under *simplicity first, then performance*.
- Deprecating the Go-layout `format`/`parse`.
