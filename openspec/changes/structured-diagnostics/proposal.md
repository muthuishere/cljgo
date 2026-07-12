## Why

ADR 0015 (docs/adr/0015-structured-diagnostics-introspection.md, status
proposed) makes every diagnostic a structured value with a stable error code,
machine-applicable fixes, and a debug-only compiler introspection API —
because editors, CI, and above all LLM agents must consume diagnostics
directly, not parse prose. It starts M2 (the fixes[] machinery is a
forcing-function on every diagnostic written from M2 onward) and the API
grows M2→M5. Owner constraint: ON TOP of existing behavior — human-readable
errors unchanged by default.

## What Changes

- One internal diagnostic data model (shared with analyzer AST/positions —
  no parallel bookkeeping) rendered two ways: human text (default,
  unchanged) and JSON via `--json` on every CLI verb.
- JSON schema per ADR 0015's field list: error_code, severity, message,
  location {file,line,column,end_line,end_column}, expected/found, fixes[]
  {title, replacement, byte_range}, related[] notes.
- Error-code registry with R/A/E/I bands (reader/analyzer/emitter/interop),
  append-only policy enforced by test, one explain page per code — location
  settled in design.
- Debug-mode introspection API, never in production binaries or the default
  REPL: compiler.check / explain / suggest_fix / get_ast / get_symbols ship
  first; get_types / get_control_flow / get_data_flow are structured
  "not yet available" stubs (ship-vs-stub split settled in design).
- Two transports, one schema: a `clojure.compiler` namespace inside the
  debug REPL, and JSON over stdio for external tools and agents.

## Non-goals

- No change to default human-readable error text or format.
- No LSP or nREPL implementation — they become thin adapters later, over
  this API (ADR 0015 §3); not in this change.
- No socket transport in v1 — stdio only (design settles framing; socket is
  a follow-up).
- No auto-applied fixes: fixes[] are data; applying them is the consumer's
  act.
- No warning-code sweep of existing behavior beyond errors the touched
  paths already raise; new codes are added as diagnostics are written.

## Capabilities

### New Capabilities
- `diagnostics`: the structured diagnostic value, JSON schema, `--json`
  wiring, error-code registry + append-only policy, explain pages.
- `compiler-introspection`: the debug-gated introspection endpoints over the
  clojure.compiler namespace and stdio-JSON transports.

### Modified Capabilities
(none — openspec/specs/ has no existing capability this touches)

## Impact

- ADRs relied on: **0015** (implemented here), 0002 (one analyzer feeds both
  renderers and the API), 0004 (debug-only gating keeps the perf mandate),
  0005 (I-band interop errors). Design authority: design/03 §1 (AST node
  design + positions — the owning data model), design/01 §3 (reader Go
  package + error positions), design/00 §3 (pkg layout for the new pkg/diag).
- Code: new pkg/diag (model, registry, renderers), pkg/reader + pkg/analyzer
  (+ pkg/emit from M2) emit through it, cmd/cljgo (`--json`, `debug` verb,
  stdio server), pkg/repl (debug-REPL namespace), docs/diagnostics explain
  pages.
- Coordination: the testing-first-class change's `--json` output and the
  result-option lint consume this schema; M2 emitter diagnostics
  (m2-emitter-v0 if open — referenced, never edited) get E-band codes.
