# builtins-to-lang

## Why

ADR 0023's structural fix (emitted binaries drop the interpreter) and
ADR 0042's consequences both name the same prerequisite: the ~300
Go-native clojure.core builtins live in `pkg/eval`, so any code that
needs them (including a future AOT-compiled core.clj) must link the
whole tree-walk interpreter. This is AOT-core **piece 2**: move every
interpreter-independent builtin out of `pkg/eval` so compiled core can
reference builtins without dragging `pkg/eval` into the link. Audit
(2026-07-17, all 20 builtin-registering files + builtins.go): of ~305
registered builtins only **5** genuinely reach evaluator state —
`macroexpand-1`, `macroexpand` (the macro engine), `eval`
(Evaluator.EvalForm), `require` (LibLoader/file loading), `require-go`
(per-evaluator host aliases). Everything else is pure
`func(args ...any) any` over `pkg/lang` (+ `pkg/reader` value types +
`pkg/version`).

## What Changes

- New package **`pkg/corelib`** (ADR 0043: not `pkg/lang/builtins` —
  pkg/lang is vendored-Glojure provenance territory) housing the pure
  builtins. It imports `pkg/lang`, `pkg/reader`, `pkg/version` and
  stdlib ONLY — never `pkg/eval`/`pkg/analyzer`/`pkg/ast`, enforced by
  a `go list -deps` test.
- `corelib.Def(name, fn) *lang.Var` / `corelib.DefPrivate` is the
  registration seam (the nativeFn wrapper moves with it);
  `corelib.RegisterAll()` interns the full pure set. Idempotent the
  same way today's per-`eval.New()` re-interning is (BindRoot).
- `pkg/eval.internBuiltins` becomes: `corelib.RegisterAll()` + the 5
  coupled builtins. Symbol resolution (`Evaluator.resolveVar`) is
  stateless over the global namespace world; its body moves to
  `corelib.ResolveVar` so the protocol/class-ref substrate
  (`-type-key`, `-instance-of-name?`) and `read-string`/EDN readers
  move too (the reader resolver is equally stateless).
- `eval.Out` (the swappable print sink) moves to `corelib.Out`;
  callers (repl, nrepl, conformance, cmd) are updated mechanically.
- NO behavior change: same names, same closures, same interning order
  observable effects. The conformance dual harness + jank suite are
  the proof (byte-identical outputs, suite file count unchanged).
- `rt.Boot()` still constructs the evaluator (core.clj is still
  interpreted at boot) — cutting that edge is piece 3. Piece 2's exit
  proof is the corelib import-hygiene test.

## Impact

- Affected: `pkg/eval` (shrinks by ~20 files), new `pkg/corelib`,
  `pkg/repl`/`pkg/nrepl`/`cmd`/`conformance` (eval.Out → corelib.Out).
- Not affected: `pkg/emit`, emitted-code shape, `pkg/emit/rt` API,
  core/*.cljg, conformance expectations.
- Piece 3 unblocked: `rt.Boot()` can call `corelib.RegisterAll()`
  without an Evaluator once core.clj is an emitted package.
