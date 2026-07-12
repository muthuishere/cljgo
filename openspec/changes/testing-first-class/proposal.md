## Why

ADR 0012 (docs/adr/0012-testing-first-class.md, status accepted) makes
testing a toolchain primitive, Zig-style: `cljgo test` with zero config,
tests colocated with source as first-class, and the dual-harness
REPL-vs-binary guarantee (ADR 0002/0007) extended to USER code. The
interpreted runner can land before M2 (it only needs core/test.clj); the
compiled and `--both` paths land with the M2 emitter.

## What Changes

- New CLI verb `cljgo test`: zero-config discovery and execution of tests
  across the project — dedicated test namespaces AND `(deftest ...)` forms
  colocated with source.
- clojure.test API compatibility in core/: `deftest`, `is`, `testing`,
  `use-fixtures`, `run-tests` (surface settled in design) so existing Clojure
  test idioms port unchanged.
- Dual-harness for user code: `cljgo test` (interpreted, default),
  `cljgo test --compiled` (same tests through the AOT path), and
  `cljgo test --both` which diffs the two runs and fails on divergence —
  design settles the diff output format.
- Colocated tests eliminated from production builds: design settles the
  mechanism (emission into Go `_test.go` files, giving Go-native exclusion).
- `(defbench ...)` benchmarks-as-tests under `cljgo test --bench`, backed by
  Go's testing.B (ADR 0004: perf is a feature).

## Non-goals

- No external-runner plugin system (kaocha-style hooks, custom reporters
  beyond the built-in human + JSON output).
- No `are`, `assert-expr` extension points, or generative/property testing in
  this change — compat surface is the settled subset; the rest is follow-up.
- No test parallelism policy beyond Go's defaults in compiled mode.
- No watch mode.
- `--compiled`/`--both`/`--bench` do not land before the M2 emitter exists;
  the interpreted runner does not wait for them.

## Capabilities

### New Capabilities
- `test-runner`: the `cljgo test` verb — discovery, colocated tests,
  prod-build elimination, dual-harness `--both`, and `defbench`.
- `clojure-test-compat`: the clojure.test-compatible core API surface.

### Modified Capabilities
(none — openspec/specs/ has no existing capability this touches)

## Impact

- ADRs relied on: **0012** (implemented here), 0002/0007 (dual-mode + dual
  harness — extended outward), 0004 (benchmarks/perf), 0001 (emitted output
  is plain Go, enabling `_test.go` elimination). Design authority:
  design/00 §6 (M2 roadmap and conformance discipline — owning section),
  design/03 §7d (divergence is the unforgivable failure), design/04 §1
  (namespace→package emission that `_test.go` placement extends).
- Code: cmd/cljgo (verb + flags), core/ (test.clj), pkg/eval (interpreted
  run), pkg/emit (deftest→`_test.go`, defbench→Benchmark funcs), conformance/
  (runner behavior tests).
- Coordination: compiled path layers on the M2 emitter change
  (m2-emitter-v0 if open in openspec/changes/ — referenced, never edited).
