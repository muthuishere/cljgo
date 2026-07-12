# ADR 0012 — Testing is first-class in the toolchain, Zig-style
Date: 2026-07-12 · Status: accepted (implementation via OpenSpec, begins M2)

## Context
Zig made testing a language/toolchain primitive: tests live with the code,
`zig test` needs zero config. Clojure has clojure.test but running it well
requires external runners (kaocha, cognitect test-runner). We already run a
dual-harness conformance suite internally — users deserve the same power.

## Decision
1. `cljgo test` is a built-in CLI verb, zero config: discovers and runs tests
   across the project (test namespaces AND tests colocated with source —
   `(deftest …)` next to the fns it tests is first-class, like Zig's
   `test "…" {}` blocks; dead-code-eliminated from production builds).
2. clojure.test API compatibility (deftest/is/testing/run-tests) in core, so
   existing Clojure test idioms port unchanged.
3. **Dual-harness for user code**: `cljgo test` runs interpreted by default;
   `cljgo test --compiled` runs the same tests through the AOT path;
   `cljgo test --both` diffs them — every user gets our REPL-vs-binary
   consistency guarantee for THEIR code (ADR 0002/0007 extended outward).
4. Benchmarks are tests: `(defbench …)` runs under `cljgo test --bench`,
   backed by Go's testing.B machinery (perf-is-a-feature, ADR 0004).

## Consequences
Zero-config testing from day one of a user project; test placement is free;
the conformance discipline becomes a user-facing feature, not just ours.
Runner lands with M2 (needs the compiled path to mean anything); interpreted
runner can land earlier with core/test.clj.
