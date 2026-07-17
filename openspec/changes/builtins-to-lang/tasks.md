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
  == pre-change baseline (242/242 on this tree, 2026-07-17); emitted
  hello-world binary behavior unchanged (still links eval via rt.Boot
  — expected until piece 3).
- [x] 4.3 Piece-3 follow-ups recorded in design.md §4.

## Notes

- Measured (2026-07-17, hello-world module built unstripped): the
  emitted binary now carries 652 pkg/corelib symbols and only 155
  pkg/eval symbols (was 381 per ADR 0023) — the remaining eval/analyzer
  link is exactly the `rt.Boot() → eval.New()` edge piece 3 cuts.
- jank suite before == after: 242/242 files passing (this tree's
  baseline, ahead of the committed Batch-0 scoreboard).
- `read-string` remains REPL-unresolvable BEFORE and AFTER (only the
  private `-edn-read-string` seam exists) — no regression, and the EDN
  readers moved cleanly onto corelib's stateless NSResolver.
