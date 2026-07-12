# ADR 0004 — Per-call var deref everywhere; fixed-arity calling convention is the M2 default
Date: 2026-07-12 · Status: accepted (amends design/04 §5 ladder placement)

## Context
Owner mandate: high performance in both modes, no compromise — AND live re-def
(REPL-driven development). Spike S6: per-call atomic var deref costs ~2%
(free); the variadic func(...any) convention allocates per call → 20–22× raw
Go, FAILING the ≤10× M2 budget; fixed-arity closure fields hit 3.5–7.8×.

## Decision
Vars are always deref'd per call (never inlined; direct-linking forbidden in
eval, opt-in only in emit). Known-arity call sites emit fixed-arity function
fields; variadic Fn/Apply remains only the apply/HOF/interop fallback.

## Consequences
REPL liveness costs nothing measurable; M2's default emission meets the perf
budget. Benchmarks live in CI beside tests.
