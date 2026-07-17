# Tasks — aot-core-cutover

## 1. One boot table, one compile seam

- [x] 1.1 `core.BootSources()`: the 13 embedded sources (ns, *file*,
  text, Go pkg), in boot order. `eval.loadBootSource` replaces the 13
  `loadX` funcs; `eval.New` = `NewBare()` + the table + user ns.
- [x] 1.2 `eval.NewBare()` (builtins + defmacro only) and
  `corelib.InitUserNS()` (user refer + doc refer + *ns* root), shared
  by the interpreter and rt.Boot.
- [x] 1.3 `emit.CompileSource(ev, r, filename)` — capture through a
  caller-supplied evaluator; `emit.EmitBootPackage` (pkgSpec.bindNS:
  Load() binds *ns* to the source's namespace, as the loader does).

## 2. Generate the core

- [x] 2.1 `cmd/gencore`: NewBare → each boot source read/analyze/eval/
  capture → one Go package each under `pkg/coreaot/`, plus load.go
  (ordered Load + `rt.RegisterCoreLoader` init). `go generate`.
- [x] 2.2 Emitter gaps found by compiling core: regex literals as
  constants (per-site, not deduped — Pattern identity); rt/lang/reader
  imports only when used; "" propagation from control-transferring
  let/if bodies (go vet unreachable).
- [x] 2.3 `pkg/coreaot/generated_test.go`: regenerate + byte-diff.

## 3. Cut the edge

- [x] 3.1 `rt.Boot` = RegisterAll + snapshot + registered core loader;
  `rt.RegisterCoreLoader`; emitted main blank-imports pkg/coreaot.
- [x] 3.2 Relocate to corelib: host reflect path + exception
  normalizers. pkg/eval keeps only the analysis-time half (require-go
  aliases).
- [x] 3.3 Relocate the provider registry + require's libspec surface to
  corelib; `corelib.SetLibFileLoader` hook installed by pkg/eval.
- [x] 3.4 `pkg/corelib/aot_stubs.go`: eval/macroexpand/macroexpand-1
  throw, require-go no-ops; pkg/eval overwrites all four.
- [x] 3.5 `pkg/coreaot/imports_test.go`: no interpreter in the closure
  of pkg/coreaot or pkg/emit/rt (all-or-nothing).

## 4. Prove + measure

- [x] 4.1 Gates: build, vet, gofmt, `go test ./...`.
- [x] 4.2 Dual harness: byte-identical, with `;; harness: eval` waivers
  on eval-value / macro-macroexpand naming the AOT deviation.
- [x] 4.3 jank clojure-test-suite: no regression vs baseline.
- [x] 4.4 hyperfine startup + binary size + link-set symbol counts,
  before (origin/main) vs after, same machine — published in ADR 0046
  Consequences, including where the targets were missed.
- [x] 4.5 ADR 0046.
