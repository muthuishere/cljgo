## Context

ADR 0012 is accepted; this design settles the four open mechanisms:
discovery rules, colocated-test elimination in production builds, the
clojure.test compat surface, and the `--both` diff output format. The
internal conformance suite (design/00 §6) is the in-house precedent; this
change productizes it for users. Interpreted runner precedes M2; `--compiled`
and `--both` require the emitter.

## Goals / Non-Goals

**Goals:**
- Zero config: `cljgo test` in a fresh project just works.
- Test placement is free (colocated or separate) with zero prod-binary cost.
- Users get the REPL-vs-binary guarantee for their own code.

**Non-Goals:**
- Reporter plugins, `are`/assert-expr, property testing, watch mode,
  parallelism tuning.

## Decisions

### D1 — Discovery rules (settled)
`cljgo test` loads **every source file** under the project's source and test
paths (project file `:paths` + `:test-paths`, defaulting to `src` and `test`
when absent — and, when neither exists, the current directory tree), then
collects every var carrying `:test` metadata — exactly clojure.test's model,
so discovery is metadata-driven, not filename-driven. Consequences: colocated
deftests are found for free (they load with their namespace); no `-test`
suffix requirement (a convention, not a rule); load order = require order
per design/03 §3b. `--ns <sym>` / `--var <sym>` filter flags narrow the run.
Alternative rejected: filename globbing (`*_test.clj` only) — breaks the
colocated-first-class mandate of ADR 0012 decision 1.

### D2 — Colocated-test elimination in prod builds (settled)
The emitter routes every top-level `deftest`/`defbench` form (and forms
marked `^:test-only`) into a **sibling `<file>_test.go` file** in the same
emitted package (extends design/04 §1 namespace→package layout). Go's own
build model then guarantees elimination: `go build` never compiles
`_test.go`. `cljgo test --compiled` becomes `go test` over the emitted tree.
Alternatives rejected: conditional AST stripping (a second code path to keep
consistent — 7d risk) and linker DCE reliance (deftest registers vars via
side effect in Load(), so DCE alone cannot remove them). One consequence to
verify against Go source, not priors: test files may reference private
package identifiers — same package, so yes. Var-registration for `run-tests`
inside compiled tests happens in a test-only `TestMain` shim emitted into
the `_test.go` file.

### D3 — clojure.test compat surface (settled)
Ships: `deftest`, `is` (with `=`-form and thrown? special cases), `testing`,
`use-fixtures` (:each/:once), `run-tests`, `run-all-tests`, `*test-out*`,
`successful?`, report-map shape per clojure.test (:pass/:fail/:error
counts). Every compat behavior gets a conformance .clj whose expectation is
frozen against real JVM clojure.test 1.12.5 output (the oracle). Deferred:
`are`, `assert-expr`/custom `report` multimethod extension, fixtures
composition edge cases beyond documented ones.

### D4 — `--both` diff output format (settled)
Runs interpreted first, then compiled, on the same discovered test set.
Per-test comparison key: fully-qualified test var. Compared fields: outcome
(pass/fail/error), assertion counts, and the ordered list of failure
messages (normalized: absolute paths stripped to project-relative,
gensym/addresses masked). Output: a human table listing ONLY divergent tests
as `DIVERGE <ns>/<test>: interpreted=<outcome a/p counts> compiled=<outcome>`
plus a trailing summary line `dual-harness: N tests, M divergences`; exit
code non-zero iff M > 0 OR either run itself failed. `--json` emits the same
comparison as structured diagnostics (schema owned by the
structured-diagnostics change — this change consumes it, one schema, and
falls back to a documented ad-hoc JSON shape if that change hasn't landed).
Rationale: divergence-only output keeps the signal identical to our internal
conformance discipline (design/03 §7d — divergence is a release blocker).

### D5 — defbench (settled)
`(defbench name & body)` registers a benchmark var; `cljgo test --bench`
runs benchmarks (not tests): interpreted mode times body iterations with a
simple harness (numbers are indicative); compiled mode emits
`func Benchmark<Munged>(b *testing.B)` into `_test.go` and defers to
`go test -bench` — the CI-budget-grade numbers (ADR 0004).

## Risks / Trade-offs

- [Loading all source at discovery executes top-level side effects] → same as require semantics; documented; `--ns` narrows.
- [Message normalization too loose/strict → false diffs or missed ones] → normalization rules are themselves conformance-tested.
- [`_test.go` shim TestMain conflicts with user Go tests in mixed repos] → emitted packages are cljgo-owned (design/04 §1); documented constraint.

## Open Questions

- Interpreted `--bench` harness minimum viable statistics (mean only vs Go-style adaptive N) — decided at implementation, budget per ADR 0004 unaffected.
