# ADR 0034 — Kill the runtime.Stack() goroutine-ID lookup on the dynamic-var hot path
Date: 2026-07-16 · Status: accepted · Evidence: spike S18 (spikes/s18-ubuntu-boot-anomaly/VERDICT.md) · Answers ADR 0024's open question

## Context

The "ubuntu boots 20× slower" anomaly is OURS, not the runner's:
allocs/op are identical across hosts and GOMAXPROCS/GOGC settings, and
CPU profiling shows **72.85% of boot time inside
pkg/lang/internal/goid.Get()** — which allocates a buffer, captures a full
runtime.Stack() trace, and text-parses "goroutine N" out of it, on EVERY
dynamic-var deref (Var.getDynamicBinding). CurrentNS() derefs the dynamic
*ns* on nearly every analyzer/eval step, so a deeply-recursive tree-walk
boot pays it constantly; runtime.Stack() cost scales with stack depth and
is OS-sensitive — a coherent mechanism for ubuntu being punished hardest.
This taxes the whole interpreter, not just boot (owner mandate: performance
is a feature).

## Decision

1. Replace goid.Get()'s stack-parse with a cheap goroutine-ID mechanism
   (the getg()-based approach used by petermattis/goid — implement per
   Go 1.26, guarded by build tags with the stack-parse as fallback for
   unsupported arches). Vendored surgery → pkg/lang/PROVENANCE.md.
2. Benchmark-verify: BenchmarkBoot -benchmem before/after locally, and
   dispatch .github/workflows/boot-bench.yml (merged with S18) on main to
   confirm the ubuntu gap collapses toward macos's ~2×.
3. If the gap closes, tighten CI's CLJGO_BOOT_BUDGET (ADR 0024's 5s was
   sized for the pathology this fixes).

## Consequences

Faster boot AND faster steady-state dynamic-var access everywhere. The
host-relative budget machinery (ADR 0024) stays — it is correct regardless.
