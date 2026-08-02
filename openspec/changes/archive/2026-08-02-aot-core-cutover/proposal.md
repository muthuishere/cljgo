# aot-core-cutover

## Why

ADR 0037 measured what ADR 0023 §2 asserted: `cljgo build` compiles the
user's forms and does NOTHING for `clojure.core`. `rt.Boot()` called
`eval.New()`, tree-walking 2980 lines of core.clj + .cljg on every
startup — an emitted binary was a native Go program whose standard
library was a set of interpreted closures rebuilt from source per run
(`reduce`: 1.00× speedup from compiling; startup ~28 ms; 155 pkg/eval
symbols in a hello-world's link set).

Pieces 1 (ADR 0042: one Go package per namespace + a provider registry)
and 2 (ADR 0043: 345 pure builtins in pkg/corelib, no-eval-import
proof) pre-paved this. Piece 3 is the cutover itself: compile cljgo's
own core through cljgo's own emitter and cut the `rt.Boot() →
eval.New()` edge, so a compiled binary links the compiled core and
never the interpreter.

## What Changes

- **`core.BootSources()`** — ONE ordered table of the embedded boot
  sources (namespace, *file*, text, Go package name). The interpreter's
  boot (`eval.New` → `loadBootSource`) and the AOT core compiler walk
  the same table in the same order; the 13 hand-written `loadX` funcs
  collapse into one loop.
- **`cmd/gencore` → `pkg/coreaot`** (checked-in generated code,
  `go generate ./pkg/coreaot`): each boot source compiled by pkg/emit
  into a Go package whose guarded `Load()` pushes the interpreter's
  load frame and runs that source's forms. `coreaot.Load()` = the table
  in order + `corelib.InitUserNS()`. A drift test regenerates and
  diffs.
- **`eval.NewBare()`** — builtins + defmacro, no core loaded: the
  compile-time seam (core.clj must analyze against exactly the
  clojure.core the interpreter gives it).
- **`rt.Boot()` = `corelib.RegisterAll()` + snapshot + the registered
  core loader.** No Evaluator. `rt.RegisterCoreLoader` inverts the
  rt↔coreaot edge (coreaot's init registers; emitted `main`
  blank-imports coreaot so the linker keeps it).
- **Relocations to pkg/corelib**: the reflect interop path
  (CallGoMethod/GoFieldGet/GoFieldSet/MakeGoStruct/NewGoStruct + the
  shaping table), the exception normalizers (Throw/Recover/
  CatchMatches), and the lib-provider registry + the whole `require`
  libspec surface. `require`'s source-file half becomes a hook
  (`corelib.SetLibFileLoader`) the interpreter installs — so a compiled
  binary gets a real, registry-backed require, and the coupled-builtin
  count drops from 5 to 4.
- **The 4 analyzer-coupled builtins in AOT** (`pkg/corelib/aot_stubs.go`,
  overwritten by pkg/eval when an evaluator exists): eval /
  macroexpand / macroexpand-1 are BOUND to a stub that throws "not
  available in an AOT-compiled binary"; `require-go` is a NO-OP (a
  compile-time directive; the emitter already linked those calls).
  Documented deviation — see ADR 0046 §5.
- **Emitter**: regex literals emit as constants; let/if bodies that
  transfer control propagate "" (no dead code — `go vet` gates the
  now-in-repo generated packages); `main` blank-imports pkg/coreaot.
- **CI proof**: `pkg/coreaot/imports_test.go` — `go list -deps` of
  pkg/coreaot and pkg/emit/rt contains no pkg/eval / analyzer / ast /
  emit / repl. All-or-nothing: one edge relinks everything.

## Impact

- Measured (darwin/arm64, same hello-world, before = origin/main
  084a6e0): startup **28.4 ms → 6.0 ms** (hyperfine, 200+ runs; Go
  floor 1.5 ms); stripped binary **5.34 MB → 4.59 MB**; link set
  **pkg/eval 155 → 0**, pkg/analyzer 63 → 0, pkg/ast 14 → 0.
  Size did NOT reach ADR 0023 §2's ~2 MB — the tree-walker is gone but
  ~13k lines of generated core arrived, and pkg/lang + corelib + reader
  dominate what is left. Reported, not spun (ADR 0046 Consequences).
- Interpreter/REPL behavior unchanged; dual harness stays byte-identical
  except the 2 files that exercise eval/macroexpand, which carry
  `;; harness: eval` waivers naming the deviation.
- keel stays interpreted (lazy registry namespaces; pkg/keel imports
  pkg/eval) — an AOT keel app is separate work.
