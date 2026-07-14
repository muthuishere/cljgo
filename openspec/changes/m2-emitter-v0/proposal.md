# m2-emitter-v0 — first emitted Go binary (milestone M2)

## Why

M2 of design/00-architecture.md §6: `cljgo build examples/hello/core.clj &&
./hello` must print from a static binary with startup < 50 ms, and the
conformance suite must run dual-harness (eval AND compiled, byte-identical
output) from here on. The emitter is the second consumer of the one analyzer
(ADR 0002) and the reason the project exists (ADR 0001: `.go` IS our IR).

## What changes

- **New `pkg/emit`**: analyzed AST (all current ops) → flattened Go source
  text → `go/format.Source` gate → a generated, self-contained Go module →
  `go build`. Ports the S1/S5-validated techniques verbatim: statement
  flattening with temp vars (no IIFEs), labeled `for {}` + `continue` recur,
  simultaneous rebinding via temps, per-iteration copies + binding-var/
  carrier split for closure-captured recur carriers (the S5 fix), `_ = x`
  discipline, hoisted package-level keyword/symbol/var interns (design/00
  §4.4), guarded source-ordered `Load()`, `main` emission.
- **Calling convention per ADR 0004**: every var reference derefs per call
  (atomic load, never inlined); single-fixed-arity fns (arity ≤ 4) emit as
  `lang.FnFunc0..4` so known-arity call sites through `lang.Apply0..4` hit
  the fixed-arity fast path with zero `[]any` allocation (the S6-winning
  shape); variadic `lang.FnFunc`/`Apply` remains the multi-arity/variadic/
  HOF fallback.
- **Munging scheme** documented as `pkg/emit/MUNGING.md` — chosen consciously
  because ADR 0013 makes it a public contract from M2.
- **Runtime bootstrap (pragmatic v0 per design/04)**: macros are already
  expanded by the analyzer; the emitted binary calls `rt.Boot()`
  (pkg/emit/rt) at startup — it constructs the evaluator so builtins +
  embedded core.clj vars resolve, and snapshots the pristine builtins for
  the guarded arithmetic intrinsics (see design.md). AOT-compiling core.clj
  itself is M5.
- **`cmd/cljgo`**: new `build` verb (`cljgo build <file.clj> [-o out]`).
- **`conformance/`**: dual harness — every `tests/*.clj` also compiles and
  runs through the emitter unless marked `;; harness: eval` (with reason);
  byte-identical output required (ADR 0007). New `ORACLE=1` mode re-audits
  frozen expectations against the real `clojure` CLI (1.12.5), honoring
  `;; oracle: skip` markers.
- **`examples/hello/core.clj`**: the M2 demo program.

## Non-goals

- Go interop (`:require-go`, go/packages signatures) — M3 (design/04 §7 v0
  lists it, but design/00 §6 places interop in M3; this change follows 00).
- Multiple namespaces / `:require` chaining, deps pinning — v0.5.
- try/catch/throw, deftype/defprotocol — v1; performance ladder beyond the
  ADR 0004 default — post-M2.
- AOT-compiling core.clj (M5); `--lib`/`--c-shared` buildmodes (ADR 0013,
  own change).
- Emitting fixed-arity fast paths for multi-arity/variadic fns (documented
  fallback to variadic `FnFunc`).

## ADRs relied on

ADR 0001 (emit Go source, format.Source gate), ADR 0002 (one analyzer, two
consumers — emitter consumes M0–M1 ASTs with zero re-analysis), ADR 0004
(per-call var deref + fixed-arity default emission), ADR 0007 (dual harness
+ ORACLE mode), ADR 0013 (munging = public contract). Owned contracts:
design/00 §4.1–4.4, design/04 §1/§3/§4/§6/§7.
