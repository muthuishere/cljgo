---
title: Compatibility
description: How cljgo measures Clojure compatibility — a dual-harness conformance suite oracle-cited against JVM Clojure 1.12.5, plus the jank clojure-test-suite as an external yardstick.
---

cljgo does not self-grade compatibility. It is measured two ways: an internal
conformance suite frozen against real JVM Clojure, and an external suite the
project does not control.

## The conformance discipline

Every semantic behavior in cljgo is a plain `.clj` file under
[`conformance/tests/`](https://github.com/muthuishere/cljgo/blob/main/conformance/README.md)
with a frozen `;; expect:` output. Each expectation is verified against **real
JVM Clojure 1.12.5** (the `clojure` CLI is the semantic oracle, needed at
authoring time only) and cited in a comment.

Two properties make this stricter than a typical test suite:

- **Dual harness.** Every file runs through *both* execution paths — the
  tree-walk evaluator (the REPL) *and* the AOT-compiled binary — and must
  produce identical output on both. The README counts **416 oracle-cited
  files** enforced on every commit.
- **Divergence is a release blocker.** REPL-vs-binary divergence is treated as
  the one unforgivable failure mode. A file that cannot run both ways needs a
  written waiver in the file itself.

The file format is simple: any number of forms evaluated in order in a fresh
`user` namespace, with exactly one expectation comment — `;; expect: <text>`
(the `pr-str` of the last form's value must match exactly) or
`;; expect-error: <text>` (the error message must contain the text).

Why divergence is structurally hard in the first place is covered in
[Architecture](/cljgo/reference/architecture/): one reader, one analyzer, one
AST feed both backends.

## The external yardstick: jank's clojure-test-suite

cljgo is also scored against the
[jank clojure-test-suite](https://github.com/jank-lang/clojure-test-suite)
(upstream @164a4b3, unmodified) — 242 real `clojure.core` test files — as a
single ratcheting number in CI. The passing count may only rise.

| Measure | Result |
|---|---|
| `clojure.core` vars resolved | 242 / 242 (100%) |
| Suite files fully passing | **238 / 242 (98.3%)** |
| Failures | 0 |
| Errors | 4 |

Reproduce it yourself: `cljgo suite`. The suite runs interpreted; cljgo's own
dual-mode conformance suite stays fully green alongside it.

### The 4 outstanding files, honestly

The four errors (`abs`, `add-watch`, `short`, `reduce`) are **dialect
registration, not broken semantics**. Those files carry reader conditionals
with no `:default` branch (e.g. `#?(:cljr System.Int16 :clj java.lang.Short)`),
so a runtime the suite has never heard of reads them as nothing — and
`(instance? (short 0))` then fails with "wrong number of args (1)".

Adding a `:cljgo` branch is the same mechanism `:cljr` / `:lpy` / `:phel`
already use, and cljgo's spellings are truthful
(`(instance? java.lang.Short (short 0))` is genuinely `true` here, as on the
JVM). With those four branches applied the suite reads **242/242 (100%)** —
but they are **not upstreamed yet**, so the published number is the one you get
from the suite as it ships. Full analysis:
[`docs/suite-upstream.md`](https://github.com/muthuishere/cljgo/blob/main/docs/suite-upstream.md).

## What is and isn't complete

`clojure.core` itself is not yet complete — the honest per-namespace ledger is
[`docs/fundamentals-audit-2026-07.md`](https://github.com/muthuishere/cljgo/blob/main/docs/fundamentals-audit-2026-07.md).
Complete against the 1.12.5 oracle (per the README, satellite rows re-verified
2026-07-23):

- Satellite namespaces: `clojure.string`, `set`, `edn`, `walk`, `zip`, `data`,
  `repl`, `pprint`; `clojure.test` complete (39 oracle vars).
- `clojure.core.async`: 55 publics — every non-deprecated, non-internal var of
  JVM core.async 1.6.681, over real goroutines.
- JVM-compatible hashing (ADR 0051), the numeric tower
  (bigint/bigdec/ratios/promotion), transients, tagged literals and reader
  conditionals.

Where Go forces a deviation (typed nil, no STM, RE2 regex), the design doctrine
is to document it loudly rather than approximate silently — see
[`design/00-architecture.md`](https://github.com/muthuishere/cljgo/blob/main/design/00-architecture.md).
