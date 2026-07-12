# ADR 0015 — Structured JSON diagnostics + debug-mode compiler introspection API
Date: 2026-07-12 · Status: proposed (owner-directed; design via OpenSpec; diagnostics begin M2, API grows M2→M5)

## Context
Free-form error text forces every consumer — editors, CI, and above all LLM
agents — to parse prose. Rust/Elm proved structured diagnostics with codes
and fixes transform tooling; LLM agents make it mandatory: a model should
consume diagnostics directly, no interpretation. Our errors already carry
positions (reader/analyzer CompilerError) — this ADR makes that machine-
grade. Owner constraint: ON TOP of existing behavior — human-readable
Clojure-style errors unchanged by default.

## Decision
1. **Every diagnostic is a structured value internally**, rendered two ways:
   human text (default, unchanged) or JSON (`--json` on any CLI verb; also
   the API below). Schema (per the owner's sketch):
   error_code (stable registry, e.g. E1002; R=reader/A=analyzer/E=emitter/
   I=interop bands), severity (error|warning|note), message, location
   {file,line,column,end-*}, expected/found where applicable, fixes[] with
   {title, replacement, byte-range} (machine-applicable), related[] notes
   (e.g. "expression starts here", "recur target is this loop").
   Codes are append-only and documented; each code has an explain page.
2. **Debug-mode compiler introspection API** — available ONLY under
   `cljgo debug` / --debug (never in production binaries or default REPL):
   compiler.check(source) → diagnostics[];
   compiler.explain(error_id) → the long-form doc for a code;
   compiler.suggest_fix(error_id) → fixes[] for a concrete diagnostic;
   compiler.get_ast(form|file) → the analyzed AST as data;
   compiler.get_symbols() → namespaces/vars/locals in scope;
   compiler.get_types() → inferred/hinted type facts (grows with emitter);
   compiler.get_control_flow() / get_data_flow() → graphs as data (grow
   M2→M5; stubs return "not yet available" as a structured answer).
   Transport settled in design: same surface exposed as (a) a clojure.compiler
   namespace inside the debug REPL and (b) JSON over stdio/socket for
   external tools & agents — one schema, two transports.
3. Diagnostics and introspection share the SAME data model as the analyzer
   (AST nodes, positions) — no parallel bookkeeping; nREPL and future LSP
   are thin adapters over this API.

## Consequences
LLM agents and editors consume cljgo natively: check → explain → apply fix
as a loop with byte-accurate ranges. The error-code registry becomes a
public, versioned contract (append-only). Debug-only exposure keeps the
production surface clean and the perf mandate intact (ADR 0004). The fixes[]
machinery forces us to design errors WITH remedies from M2 onward — a
quality forcing-function on every diagnostic we write.
