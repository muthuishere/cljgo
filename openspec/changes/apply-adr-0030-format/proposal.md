## Why

ADR 0030 (docs/adr/0030-format-translate-then-delegate.md, accepted) settles
`format`'s implementation strategy on the evidence of spike S14
(spikes/s14-format-grammar, 80/80 against the real JVM Clojure 1.12.5
oracle): translate-then-delegate over a direct interpreter, ~90 fewer lines
for the same measured compatibility. `format` and `printf` do not exist in
cljgo today; `conformance/tests` has no format coverage and the upstream
suite's `format.cljc` is currently unexercised. This change productionizes
the ADR's decision.

## What Changes

- New `format` core fn: Java `java.util.Formatter` grammar (conversions
  `s S d x X o c C b B f e E g G n %`, flags `- + (space) 0 , ( #`, width,
  precision, argument indexing `%N$`/`%<`), implemented translate-then-
  delegate (parse once, delegate the fmt.Sprintf-compatible core, hand-write
  `%b`/`%g`/`,`/`(`/`%n`/indexing — the ADR's bucket split).
  `%q`/date-time/anything unrecognized throws (an
  `UnknownFormatConversionException`-shaped message), matching real
  Clojure/Java exactly rather than passing the directive through unrendered.
- New `printf` core fn: `(printf fmt & args)` = `(print (format fmt args...))`
  — no separate grammar, writes through the same `eval.Out` path `println`
  already uses (design/03 §8), so it composes with whatever owns `*out*` /
  `with-out-str` without a second write surface.
- A strict per-verb Go-`fmt`-flag allow-list before any directive reaches
  `fmt.Sprintf`, so an unsupported flag/verb combination throws a typed
  format error instead of leaking `%!d(BADFLAG)`-style noise into output
  (the ADR's stated discipline cost of translate-then-delegate).
- `conformance/tests/format-*.clj`: the spike's 80-probe corpus (re-verified
  against the real `clojure` CLI 1.12.5 at freeze time, not trusted from the
  spike's recorded numbers), split into passing-value files (`;; expect:`)
  and throwing files (`;; expect-error:`), dual-harness (interpreted + AOT).
  This flips the upstream `clojure-test-suite/test/clojure/core_test/
  format.cljc` assertions cljgo runs today from skipped/erroring to passing.

## Non-goals (ADR 0030's recorded tail, not this change)

- `%f`/`%e`/`%g` rounding mode stays Go's half-to-even (documented ADR
  divergence from Java's HALF_UP at exact tie cases) — not fixed here.
- `BigDecimal`/`with-precision` arg support for `%f`/`%e` — no
  `with-precision` form exists in `core/` yet; when it lands it needs its
  own arg-kind branch, tracked as a follow-up, not blocking this change.
- `%t`/`%T` date-time conversions — out of scope per the spike; throw like
  any other unrecognized conversion.
- `cl-format` (pprint's `~a ~s ~d` grammar) — explicitly a different engine,
  not touched.
- No `*out*`/`with-out-str` work — `printf` reuses the existing `eval.Out`
  write path `println` already has; another change owns dynamic `*out*`.

## Capabilities

### New Capabilities
- `format-printf`: `format`/`printf` core fns per the Java Formatter grammar
  subset above, identical behavior interpreted and AOT-compiled.

### Modified Capabilities
(none — openspec/specs/ has no existing capability this touches)

## Impact

- ADRs relied on: **0030** (implemented here, ratifies spike S14).
- Code: new `pkg/eval/format_builtins.go` (parser + translate-then-delegate
  renderer + `format`/`printf` registration), one line added to
  `internBuiltins` (design/03 §8 wiring convention already used by every
  other builtins file).
- Conformance: new `conformance/tests/format-*.clj` (oracle-cited against
  real `clojure` 1.12.5), dual-harness from M2 — a compiled-mode run is
  spot-checked as part of this change.
- No changes to the reader, analyzer, or emitter: `format`/`printf` are
  plain `clojure.core` Vars bound to Go `IFn` closures, the same shape as
  every existing builtin (`println`, `pr-str`, `str`), so AOT compilation
  needs no new emitter intrinsic.
