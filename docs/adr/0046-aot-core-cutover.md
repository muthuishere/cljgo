# ADR 0046 — Compiled binaries link the compiled core, not the interpreter

Date: 2026-07-17 · Status: accepted · Executes **ADR 0037** (AOT-core is a
performance decision) / ADR 0023 §2, on the rails laid by **ADR 0042**
(multi-namespace emission) and **ADR 0043** (pkg/corelib). AOT-core **piece 3**.

## Context

ADR 0037 measured the thing ADR 0023 had only asserted: `cljgo build`
compiles the user's forms and does **nothing** for `clojure.core` —
`rt.Boot()` called `eval.New()`, which tree-walked 2980 lines of core.clj +
.cljg on **every startup**, so an emitted binary was a native Go program
whose standard library was a set of interpreted closures rebuilt from source
on each run (`reduce`: 1.00× speedup from compiling; startup ~28 ms).

Pieces 1 and 2 pre-paved the cutover: one Go package per namespace with a
provider registry (0042), and 345 pure builtins in `pkg/corelib` with a
CI-enforced no-eval-import proof (0043). What remained was exactly the
`rt.Boot() → eval.New()` edge — measured at the start of this change as
155 `pkg/eval` symbols (+63 analyzer, +14 ast) in a hello-world's link set.

## Decision

1. **cljgo's own core is compiled by cljgo's own emitter, ahead of time,
   and checked in.** `cmd/gencore` compiles every entry of the new ONE
   table `core.BootSources()` — core.clj, numeric, hierarchies, predicates,
   transducers, protocols (all → clojure.core), then string / set / edn /
   test / build / portability / repl — into `pkg/coreaot/<pkg>/`, one Go
   package per source, each a guarded `Load()` that pushes the same
   `*ns*`/`*file*` frame the interpreter's loader pushes and then runs that
   source's top-level forms in source order. `pkg/coreaot.Load()` calls them
   in table order and ends with `corelib.InitUserNS()`.

   The compile IS the interpreter's boot, captured: one `eval.NewBare()`
   (builtins + defmacro, nothing of core.clj yet), then each source
   read → analyzed → **evaluated** → captured, so compile time = eval time
   (ADR 0002) holds across the whole boot exactly as it does at runtime.
   The interpreter and the generator walk the SAME table in the SAME order,
   so the two namespace worlds cannot drift by construction; a
   regeneration-diff test (`pkg/coreaot/generated_test.go`) enforces that
   the committed output matches the sources it came from.

2. **`rt.Boot()` never constructs an Evaluator.** It is now
   `corelib.RegisterAll()` → snapshot the pristine arithmetic builtins →
   the registered core loader. rt cannot import `pkg/coreaot` (the
   generated packages import rt for the intrinsics), so the edge is
   inverted through `rt.RegisterCoreLoader`, called from coreaot's
   `init()`; emitted `main` blank-imports `pkg/coreaot`, which is what makes
   the linker keep it. Same shape as ADR 0042's namespace providers.

3. **The interpreter-only shims move to `pkg/corelib`**: the reflect
   interop path (`CallGoMethod` / `GoFieldGet` / `GoFieldSet` /
   `MakeGoStruct` / `NewGoStruct` + the shaping table) and the exception
   normalizers (`Throw` / `Recover` / `CatchMatches`). Both modes already
   shared these functions — only their address changes, so byte-identity is
   preserved by construction. Only the analysis-time half of interop
   (`require-go` alias resolution, which reads per-evaluator state) stays in
   `pkg/eval`.

4. **`require` is not interpreter-coupled and stops pretending to be.** The
   libspec surface (`:as` / `:refer` / prefix lists) and the provider
   registry move to `pkg/corelib`; the only interpreter-only half — making a
   namespace exist by READING ITS SOURCE FILE — becomes a hook
   (`corelib.SetLibFileLoader`) that `pkg/eval` installs. A compiled binary
   therefore gets a real, registry-backed `require`; a require that resolves
   to neither an existing namespace nor a registered provider fails with an
   error that names the AOT limitation instead of a nil-map mystery.
   The five coupled builtins are now **four**.

5. **The four that genuinely need the analyzer are bound, in AOT, to
   honest answers** (`pkg/corelib/aot_stubs.go`; `pkg/eval` overwrites all
   four through the same `Def` seam, so interpreted behavior is untouched):

   - `eval`, `macroexpand`, `macroexpand-1` throw
     "… is not available in an AOT-compiled binary".
   - `require-go` is a **no-op**: it is a compile-time directive for the
     analyzer, and by the time a binary replays it the emitter has already
     resolved and linked those calls. (Erroring here broke every AOT interop
     program — `pkg/build`'s websocket example caught it.)

   **Bound-and-throwing, not unbound.** An unbound var reports "cannot call
   unbound var: Unbound: #'clojure.core/eval", which reads like a broken
   boot; the stub names the real constraint. Referencing the var (`resolve`,
   `bound?`, `#'eval` as a value) also keeps behaving as in the REPL — only
   CALLING it fails, and it fails legibly.

   **This is a deviation, and the oracle cannot bless it.** Clojure 1.12.5
   (2026-07-17): `(eval (list '+ 1 2))` => 3 in AOT-compiled code too,
   because a JVM program always links clojure.jar — Compiler and all. There
   is no JVM artifact that lacks the compiler, so the JVM has no opinion
   about a compiler-less binary. cljgo follows the **CLJS model** (ADR 0001,
   design/00): the compiler is a build-time tool and the binary is plain Go.
   ClojureScript answers the same question the same way — `eval` is simply
   not in `cljs.core`, macroexpansion is compile-time only, and runtime eval
   is a separate opt-in artifact (self-hosted). Ours is that answer with a
   better error message. The two conformance files that exercise these names
   (`eval-value`, `macro-macroexpand`) carry a `;; harness: eval` waiver
   stating exactly this.

6. **The no-interpreter property is CI-enforced, all-or-nothing**
   (`pkg/coreaot/imports_test.go`): `go list -deps` of `pkg/coreaot` and
   `pkg/emit/rt` must contain no `pkg/eval`, `pkg/analyzer`, `pkg/ast`,
   `pkg/emit`, `pkg/repl`. One edge back is enough for the linker to keep
   everything, so the test fails on the first edge rather than on a
   symbol-count threshold.

## Consequences

Measured, same machine (darwin/arm64), same hello-world, before = origin/main
(084a6e0), after = this change:

| | before | after |
|---|---|---|
| startup (hyperfine, 200+ runs) | 28.4 ms ± 0.7 | **6.0 ms ± 0.5** |
| binary (stripped, as `cljgo build` ships it) | 5.34 MB | **4.59 MB** |
| binary (unstripped) | 7.85 MB | 6.98 MB |
| `pkg/eval` symbols in the link set | 155 | **0** |
| `pkg/analyzer` / `pkg/ast` symbols | 63 / 14 | **0 / 0** |

- **Startup: 4.7× faster, and the interpreter is gone — but 6.0 ms is not
  the ~2 ms process floor.** A do-nothing Go binary on this machine is
  1.5 ms; the remaining ~4.5 ms is `corelib.RegisterAll()` + the compiled
  core's `Load()` building ~300 vars and closures — real work, now native
  instead of interpreted. Cutting it further is a **separate** decision with
  a parity cost: today a compiled binary eagerly loads all 13 boot sources
  because the interpreter does, so `(all-ns)` and `(find-ns 'clojure.test)`
  agree in both modes. Making the satellite namespaces (clojure.test,
  cljgo.build, portability, repl) lazy through the provider registry would
  buy most of the remainder and must be argued on its own merits, not
  smuggled in here.
- **Size: 14% off the stripped binary, NOT the ~2 MB ADR 0023 §2 hoped
  for.** Honest accounting: we deleted the tree-walker but added ~13k lines
  of generated Go for core itself. ADR 0023's estimate assumed the
  interpreter was the bulk; the measurement says the runtime (pkg/lang +
  corelib's 696 symbols + pkg/reader) dominates, and it is still all there —
  a compiled binary still needs the data structures, the numeric tower, the
  printer and the reader (`read-string` is a real core fn). Size beyond this
  is a dead-code problem in `pkg/lang`/`pkg/corelib`, not an AOT-core
  problem; ADR 0023 §2's "~2 MB" should be read as superseded by this
  measurement.
- The emitter gained: regex literals as constants (`&reader.Regex` per
  literal site — deliberately not deduped, matching Pattern's identity
  semantics), and `""`-propagation from let/if bodies that transfer control
  (dead code the Go vet gate rejects once generated code lives in-repo).
  Both are behavior-preserving and proven by the dual harness.
- `pkg/coreaot` is generated code in the published module (ADR 0028): it
  ships, it is vetted and gofmt-ed like everything else, and it regenerates
  with `go generate ./pkg/coreaot`.
- Non-goals: keel stays interpreted (its namespaces load lazily through the
  registry, and `pkg/keel` imports `pkg/eval` — an AOT keel app is its own
  piece of work); no lazy core; no direct-linking of core calls (design/04
  §5's ladder is still orthogonal).
