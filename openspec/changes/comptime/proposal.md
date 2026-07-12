## Why

ADR 0009 (docs/adr/0009-comptime.md, status proposed) mandates a Zig-style
`comptime` alongside unchanged Clojure macros: ordinary Clojure evaluated once
at compile time, its **value** (not a form) embedded in the artifact as a
literal constant. The 80% macro use case — precompute a value — drops to zero
ceremony, and no other Clojure (JVM/CLJS/Glojure/jank) offers value-level
comptime with embedded-asset semantics. The machinery is nearly free: the AOT
compiler already links the tree-walk evaluator for macros.

## What Changes

- New special forms `comptime`, `comptime-assert`, and `embed-file`, analyzed
  by the shared analyzer (dual-mode, ADR 0002).
- AOT path (pkg/emit): evaluate the body at compile time, verify the result
  against an embeddability rules table, emit it as a Go literal constant in
  the namespace `Load()` (design/04 §1 "Top-level forms → one Load()", the
  owning contract; literal emission owned by pkg/emit per design/00 §3).
- Interpreted/REPL path (pkg/eval): compile time = eval time; the body
  evaluates inline with identical semantics (design/03 §7d dual-mode
  consistency — the owning acceptance contract).
- Non-embeddable results (fns, Go handles, channels, other opaque values)
  are a positioned compile error.
- Build-cache honesty: a comptime that reads files (embed-file or recorded
  I/O) contributes those inputs to build-cache invalidation.
- Documentation splits guidance: macros transform syntax, comptime computes
  values (ADR 0009 §4).

## Non-goals

- No change, ever, to `defmacro` semantics, expansion order, or hygiene
  (ADR 0009 decision 1).
- No re-introduction of `#=` read-eval (rejected; embed-file is the
  disciplined replacement).
- No comptime type-level programming (Zig's comptime generics) — values only.
- No cross-namespace comptime dependency ordering beyond existing
  load/require order.
- Not scheduled before M2 completes (target post-M2; the emitter must exist).

## Capabilities

### New Capabilities
- `comptime`: compile-time evaluation forms — `comptime`, `comptime-assert`,
  `embed-file` — embeddability rules, dual-mode semantics, and build-cache
  participation.

### Modified Capabilities
(none — openspec/specs/ has no existing capability this touches)

## Impact

- ADRs relied on: **0009** (this change implements it), 0002 (dual-mode, one
  analyzer), 0001 (emit Go source). Design authority: design/04 §1 (Load()
  emission), design/03 §7d (REPL/binary consistency), design/00 §2 (pipeline).
- Code: pkg/analyzer (three new special forms), pkg/eval (inline eval),
  pkg/emit (compile-time evaluator hook, embeddability checker, literal
  emitter, cache-input recording), cmd/cljgo (build cache keying).
- Coordination: builds on the M2 emitter; if an `m2-emitter-v0` change is
  open in openspec/changes/, this change layers on top of it — referenced,
  never edited.
- Conformance: new conformance/tests/*.clj files with oracle-noted
  expectations (comptime does not exist on JVM Clojure; files document the
  deviation loudly and freeze cljgo behavior, dual-harness from M2).
