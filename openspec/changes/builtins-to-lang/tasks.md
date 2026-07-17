# Tasks — builtins-to-lang

## 1. Scaffold + core registration

- [x] 1.1 ADR 0043 (pkg/corelib placement + seams) written.
- [x] 1.2 `pkg/corelib`: def seam (nativeFn, Def, DefPrivate), shared
  helpers (oneArg/twoArgs/currentNS/setVarValue/refer*/Out/outWriter/
  stringWriter/taps), builtins.go's pure bulk as `RegisterAll()`;
  `eval.internBuiltins` = `corelib.RegisterAll()` + the 5 coupled
  builtins; `eval.Out` callers → `corelib.Out`. Gates + conformance.

## 2. Move the pure files

- [x] 2.1 git mv array/chan/coll/format(+render)/multimethod/numeric/
  predicate/seq/sorted/string/test/transient/version/volatile
  _builtins.go (+ their internal tests), drop receivers, wire into
  RegisterAll. Gates + conformance after the batch.
- [x] 2.2 var_builtins.go moves minus `eval`; ex_builtins.go's
  registerExceptionBuiltins moves (Throw/Recover/CatchMatches/evalTry
  stay in eval). Gates + conformance.

## 3. Resolution-dependent cluster

- [x] 3.1 `corelib.ResolveVar` (body of Evaluator.resolveVar) +
  `corelib.NSResolver` (body of eval's nsResolver); Evaluator methods
  delegate. protocols.go, class_refs.go, misc_builtins.go move.
  Gates + conformance.

## 4. Proof + verification

- [x] 4.1 `pkg/corelib/imports_test.go`: `go list -deps` contains no
  pkg/eval / pkg/analyzer / pkg/ast / pkg/emit.
- [x] 4.2 Full gates green; conformance dual harness green; jank suite
  == pre-change baseline (vanilla 238/242, 2026-07-17); emitted
  hello-world binary behavior unchanged (still links eval via rt.Boot
  — expected until piece 3).
- [x] 4.3 Piece-3 follow-ups recorded in design.md §4.

## Notes

- Measured (2026-07-17, hello-world module built unstripped): the
  emitted binary now carries 652 pkg/corelib symbols and only 155
  pkg/eval symbols (was 381 per ADR 0023) — the remaining eval/analyzer
  link is exactly the `rt.Boot() → eval.New()` edge piece 3 cuts.
- jank suite (VANILLA upstream clone, main @164a4b3 — no :cljgo dialect
  patches): origin/main baseline 238/242 and this branch 238/242, with a
  per-file status diff of ZERO across all 242 files (same 4 errors: abs,
  add_watch, reduce, short — pre-existing upstream gaps, unrelated to
  this change). An earlier 242/242 reading came from a locally PATCHED
  suite branch (cljgo-dialect) and is not a publishable number.
- `read-string` remains REPL-unresolvable BEFORE and AFTER (only the
  private `-edn-read-string` seam exists) — no regression, and the EDN
  readers moved cleanly onto corelib's stateless NSResolver.
- ADR 0045 (PR #48, native reduce/map/filter/mapv/comp) landed mid-flight
  and is a semantic conflict with this change: its five new natives were
  added to pkg/eval — exactly the surface being moved. Audited against
  the same pure-vs-coupled criteria: ZERO evaluator references (the
  `(e *Evaluator)` receiver was vestigial; the bodies close over pkg/lang
  only), so hotpath_builtins.go moved to pkg/corelib with the rest and
  RegisterAll gained the one call line, in ADR 0045's original position
  (immediately after internCollBuiltins, still before loadCore so no
  core.clj defn can shadow a native). ADR 0045's laziness/transducer
  conformance tests pass in BOTH harness modes after relocation.
