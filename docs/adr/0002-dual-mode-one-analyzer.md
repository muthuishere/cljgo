# ADR 0002 — Dual mode: tree-walk evaluator + AOT emitter behind ONE analyzer
Date: 2026-07-11 · Status: accepted

## Context
Go cannot eval at runtime (no JIT; plugin is Linux-only). Clojure needs a REPL
and compile-time macro execution.

## Decision
One reader → one analyzer → one AST (Node{Op,Form,Sub}, cljs op vocabulary)
with two consumers: pkg/eval (REPL + macros) and pkg/emit (AOT, M2). Macros
always execute via the evaluator during analysis in both modes. New ops enter
analyzer + both consumers together.

## Consequences
REPL/binary divergence is made structurally hard; the conformance suite runs
every test through both paths from M2 (release blocker on divergence). The
evaluator's Clojure fidelity IS the REPL's fidelity (design/03 §7).
