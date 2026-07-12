## Context

Reader and analyzer errors already carry positions (CompilerError; design/03
§1 gives every AST node file/line/col). ADR 0015 upgrades this to a
machine-grade contract and left four things to this round: the JSON schema,
the registry file location + append-only enforcement, `--json` wiring, and
the debug-API transport + ship-vs-stub split.

## Goals / Non-Goals

**Goals:**
- Agents run check → explain → apply-fix loops with byte-accurate ranges.
- Human output byte-for-byte unchanged by default.
- One data model: diagnostics and introspection read the analyzer's own
  AST/positions (ADR 0015 §3), no parallel bookkeeping.

**Non-Goals:**
- LSP/nREPL, socket transport, auto-apply, exhaustive back-fill of codes.

## Decisions

### D1 — JSON schema (settled; ADR 0015's field list verbatim, plus versioning)
Envelope: `{"schema": "cljgo-diag/1", "diagnostics": [...]}`. Each diagnostic:

```json
{
  "error_code": "A1002",
  "severity": "error",            // error | warning | note
  "message": "recur outside loop or fn",
  "location": {"file": "src/app.clj", "line": 12, "column": 3,
                "end_line": 12, "end_column": 22},
  "expected": "recur target in tail position",   // optional
  "found": "recur in non-tail position",          // optional
  "fixes": [{"title": "wrap in loop",
              "replacement": "(loop [] ...)",
              "byte_range": {"start": 240, "end": 259}}],
  "related": [{"message": "enclosing fn starts here",
                "location": {"file": "src/app.clj", "line": 10, "column": 1}}],
  "explain_url": "docs/diagnostics/A1002.md",     // optional
  "id": "d-0001"                                   // per-run handle for explain/suggest_fix
}
```

Bands per ADR 0015: `R` reader, `A` analyzer, `E` emitter, `I` interop;
4-digit code within band. Byte ranges are offsets into the exact source
bytes the compiler read (UTF-8), making fixes[] machine-applicable without
column arithmetic. snake_case keys (JSON-consumer convention; keyword
conversion at the Clojure surface).

### D2 — Registry location + append-only policy (settled)
Source of truth: **`pkg/diag/registry.go`** — typed entries
(code/band/summary/since) so unknown codes cannot compile. Human contract:
**`docs/diagnostics/`** — `registry.md` index (generated from registry.go by
a go:generate step, drift = test failure) and one `<CODE>.md` explain page
per code (hand-written, required: a registry test fails if a code lacks its
page). Append-only enforcement: `docs/diagnostics/registry.lock` — a
generated, committed snapshot of code→summary-hash; a test fails if any
existing lock entry is removed or altered (additions append). Renumbering or
reuse is thereby structurally impossible without tripping CI. Alternative
rejected: registry-in-docs-only (no compile-time exhaustiveness, silent
drift).

### D3 — `--json` wiring (settled)
New `pkg/diag`: the Diagnostic struct, registry, and two renderers. All
CompilerError construction in pkg/reader / pkg/analyzer (and pkg/emit from
M2) flows through pkg/diag constructors (positioned, code-carrying).
cmd/cljgo gets a global `--json` flag: every verb (build/test/repl-batch/
debug) renders collected diagnostics as one D1 envelope on stderr, exit
codes unchanged; default path renders exactly today's human text (golden
tests freeze it). Cross-package contract per config rule — consumers
touched: pkg/reader, pkg/analyzer, pkg/emit, pkg/eval (runtime compile
errors), cmd/cljgo, pkg/repl.

### D4 — Debug-API transport (settled)
Gate: only under `cljgo debug` (interactive debug REPL) or
`cljgo debug --stdio` (machine transport). Never linked into emitted
production binaries; the default REPL does not expose it.
- **(a) clojure.compiler namespace** in the debug REPL: fns returning plain
  Clojure data — `(compiler/check src-or-file)`, `(compiler/explain code)`,
  `(compiler/suggest-fix diag-id)`, `(compiler/get-ast form-or-file)`,
  `(compiler/get-symbols)`, `(compiler/get-types)`,
  `(compiler/get-control-flow)`, `(compiler/get-data-flow)`.
- **(b) stdio JSON**: newline-delimited JSON requests
  `{"id":1,"method":"check","params":{...}}` → responses embedding D1
  diagnostics. NDJSON over Content-Length framing: simpler for agents/shell,
  no header parsing; LSP-style framing can wrap it later without schema
  change. One schema, two transports — the namespace fns and stdio methods
  are generated from the same endpoint table so they cannot drift.

### D5 — Ship vs stub at M2 (settled)
Ship first: `check` (analyzer already produces everything), `explain`
(reads explain pages), `suggest_fix` (returns fixes[] already attached to a
diagnostic id — the forcing-function ADR 0015 wants from M2), `get_ast`
(design/03 §1 nodes serialized to data), `get_symbols` (namespaces/vars from
design/03 §3b; locals-in-scope only for check'd source). Stub with
structured `{"error_code":"D0001","message":"not yet available",...}`:
`get_types` (grows with emitter type facts), `get_control_flow`,
`get_data_flow` (M2→M5 per ADR 0015).

## Risks / Trade-offs

- [Schema churn after agents depend on it] → `schema: cljgo-diag/1` version tag from day one; additive-only within /1.
- [fixes[] replacement drift vs byte_range on edited files] → fixes valid only against the exact bytes check'd; responses echo a source content hash.
- [Golden human-text tests brittle] → they protect the owner constraint (unchanged default); brittleness is the point.
- [AST serialization exposes internals] → get_ast marks node fields experimental in /1; only positions+ops are contract.

## Open Questions

- Whether `--json` should also stream diagnostics incrementally (NDJSON per diagnostic) for long builds — deferred; envelope-at-end for v1.
