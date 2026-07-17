# ADR 0043 — Pure builtins live in `pkg/corelib`, registered without an Evaluator

Date: 2026-07-17 · Status: accepted · Executes ADR 0023 §2 / ADR 0042 consequences (AOT-core piece 2)

## Context

ADR 0042 closed with "what remains is migrating Go builtins to
`pkg/lang` and cutting `rt.Boot()`'s `eval.New()` edge". The audit
(2026-07-17) shows ~305 registered builtins across builtins.go + 19
satellite files; only 5 (`macroexpand-1`, `macroexpand`, `eval`,
`require`, `require-go`) touch evaluator state. But "to pkg/lang"
literally is the wrong home: pkg/lang is vendored from Glojure under
EPL provenance discipline (PROVENANCE.md), while the builtins are
original cljgo code that additionally needs `pkg/reader` (the
`*reader.Regex`/`*reader.UUID` value types) and `pkg/version` —
nesting them under pkg/lang would blur provenance and create a
`lang/builtins → reader → lang` package shape.

## Decision

1. **New package `pkg/corelib`** — the Go-native half of clojure.core.
   Allowed imports: stdlib, `pkg/lang`, `pkg/reader`, `pkg/version`.
   Forbidden (test-enforced via `go list -deps`): `pkg/eval`,
   `pkg/analyzer`, `pkg/ast`, `pkg/emit`.
2. **Registration seam**: `corelib.Def(name, fn) *lang.Var` /
   `DefPrivate` intern nativeFn wrappers into clojure.core;
   `corelib.RegisterAll()` registers the entire pure set. The
   interpreter's `internBuiltins` becomes RegisterAll + the 5 coupled
   builtins; piece 3's `rt.Boot()` calls RegisterAll directly.
3. **Symbol resolution is namespace-world state, not evaluator
   state.** `Evaluator.resolveVar` and the reader's ns resolver read
   only the global `*ns*` dynamic var and the namespace registry, so
   their bodies move to `corelib.ResolveVar` / `corelib.NSResolver`
   and the Evaluator delegates. This is what lets the protocol/
   class-ref substrate and `read-string` move — compiled protocol
   code calls `-type-key` at load time and must not need the
   interpreter.
4. **No behavior change** — same closures, same names, same printing;
   proven by the conformance dual harness and an unchanged jank-suite
   scoreboard.

## Consequences

- `pkg/eval` shrinks to the actual interpreter (analyzer glue, macro
  engine, scopes, host interop, file loading); dependency packages
  emitted per ADR 0042 already import only rt + lang, so after piece 3
  they link corelib + lang, never the tree-walker.
- rt keeps importing pkg/eval until piece 3 (Boot still interprets
  core.clj; the interop/exception shims still live in eval — their
  move is recorded in the change design §4).
- The published-module surface (ADR 0028) gains pkg/corelib; it ships
  in the same module, so no packaging change.
