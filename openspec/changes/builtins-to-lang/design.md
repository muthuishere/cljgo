# Design — builtins-to-lang (AOT-core piece 2)

## 1. The audit (2026-07-17)

Every `def(...)` registration in pkg/eval was classified. Coupled =
the closure touches `*Evaluator` state (macro engine, LibLoader, host
aliases, EvalForm). Pure = closes over pkg/lang (+ reader value types
like `*reader.Regex`/`*reader.UUID`, + pkg/version) only.

| file | verdict |
|---|---|
| builtins.go | pure EXCEPT `macroexpand-1`, `macroexpand`, `require`, `require-go` |
| array/chan/coll/format(+render)/multimethod/numeric/predicate/seq/sorted/string/test/transient/version/volatile _builtins.go | pure (all `e.` hits were comments or local vars) |
| var_builtins.go | pure EXCEPT `eval` (Evaluator.EvalForm) |
| misc_builtins.go | pure once ResolveVar and the reader resolver are free functions (`-instance-of-name?`, `read-string`, EDN readers) |
| protocols.go, class_refs.go | substrate + builtins, pure under the same ResolveVar move (`-type-key`, `-qualified-name`) |
| ex_builtins.go | `registerExceptionBuiltins` pure; Throw/Recover/CatchMatches/evalTry stay in eval (not builtins; rt already imports eval until piece 3) |

Key discovery: `Evaluator.resolveVar` and the `reader.Resolver` adapter
never read evaluator fields — the "current namespace" is the global
dynamic var `*ns*` (lang.VarCurrentNS). Both become free functions in
corelib (`ResolveVar`, `NSResolver`); the Evaluator methods delegate,
so the analyzer hooks are unchanged.

## 2. Package choice: pkg/corelib (ADR 0043)

Not `pkg/lang/builtins`: pkg/lang is vendored from Glojure with EPL
provenance discipline (PROVENANCE.md scope); the builtins are original
cljgo code and also need `pkg/reader` (regex/UUID literal types) and
`pkg/version`, which would give a `lang/builtins → reader → lang`
shape. `corelib` says what it is: the Go-native half of clojure.core.

Allowed imports: stdlib, pkg/lang, pkg/reader, pkg/version.
Forbidden: pkg/eval, pkg/analyzer, pkg/ast, pkg/emit — asserted by
`pkg/corelib/imports_test.go` running `go list -deps`.

## 3. Seams

- `corelib.Def / DefPrivate` — intern a nativeFn into clojure.core
  (moved verbatim; `#object[name]` printing unchanged).
- `corelib.RegisterAll()` — registers the whole pure set; called by
  `eval.internBuiltins()` today, by `rt.Boot()` directly in piece 3.
- Exported because staying eval code uses them: `corelib.NewNativeFn`
  (defmacro bootstrap, host fns), `corelib.ReferAll` / `ReferSpec`
  (require's libspec handling), `corelib.ResolveVar` (analyzer hook
  body), `corelib.NSResolver` (reader resolver), `corelib.Out`.

## 4. Follow-ups piece 3 needs (recorded, NOT done here)

- **Cut `rt.Boot()`'s `eval.New()` edge**: replace with
  `corelib.RegisterAll()` + the compiled clojure.core package's
  registered `Load()` (ADR 0042 registry). The remaining eval imports
  in rt are the interop shims (`CallGoMethod`/`GoFieldGet`/`GoFieldSet`/
  `MakeGoStruct`/`NewGoStruct` — pkg/eval/host.go's reflect path) and
  the exception shapers (`Throw`/`Recover`/`CatchMatches`,
  pkg/eval/ex_builtins.go). None touches evaluator state; they move to
  corelib (or a pkg/hostrt) in piece 3.
- **A compiled binary still needs `require`**: emitted code replays
  `(require …)` through the var; today that builtin is interpreter-
  registered (LibLoader/file loading). Piece 3 needs a corelib-level
  require that consults ONLY the ADR 0042 provider registry (which
  must also move out of pkg/eval — it lives in pkg/eval/libload.go)
  and fails on a filesystem miss, with the interpreter overriding it
  with the file-loading version. Same pattern for the 4 other coupled
  builtins: register a "not available without the interpreter"
  stub vs. leave unbound — decide against the oracle (piece 3).
- **`defmacro` bootstrap + core.clj load** obviously stay
  interpreter-only; compiled core's `Load()` replaces them.
