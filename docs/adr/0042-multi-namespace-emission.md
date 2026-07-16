# ADR 0042 — Multi-namespace emission: one Go package per ns, registry-triggered loading, interned cross-ns var refs

Date: 2026-07-17 · Status: accepted

## Context

ADR 0023's structural fix (AOT-core: binaries drop the interpreter) needs the
emitter to produce **multiple namespaces** first — design/04 sketched this as
"v0.5": one Clojure namespace → one Go package, `Load()` chaining, `main` =
bootstrap + loads + `-main`. Until now `pkg/emit` compiled exactly one file
into one `package main`, and `require` only resolved namespaces already
embedded at boot (`loadLib` panicked otherwise).

Three design questions had to be settled, each oracled against real Clojure
1.12.5 (2026-07-17, 3-ns program `entry → multi.util → multi.data`, plus a
`cyc.a ⇄ cyc.b` cycle):

1. **When does a dependency load?** JVM Clojure loads it **at the `require`
   call site**, interleaved with the requiring file's own side effects
   (oracle: `(println "before") (require 'multi.data) (println "after")`
   prints `before` / `loading multi.data` / `after`), exactly once per
   process, and a cycle fails with `Cyclic load dependency: [ /cyc/a
   ]->/cyc/b->[ /cyc/a ]`.
2. **How does emitted code in ns A reference a var interned by ns B** —
   direct Go symbol or registry lookup?
3. **How do source files resolve?** JVM Clojure maps `my-app.util` →
   classpath `my_app/util.clj`.

design/04 §1's sketch (dep `Load()` calls at the **top** of the requiring
`Load()`) would reorder side effects relative to the interpreter — printing
`loading multi.data` before `before` — which is REPL-vs-binary divergence,
the ADR 0002/0007 release blocker.

## Decision

1. **One Clojure namespace → one Go package** under the generated module:
   ns `my-app.util` → `<gen>/my_app/util/util.go`, `package util` (segments
   munged, JVM `-`→`_` rule; last segment is the package name, deduped
   against Go keywords). The **entry** namespace's forms stay in `package
   main` (unchanged single-file shape); `main()` remains bootstrap +
   `Load()` + `-main` dispatch. Splitting the entry into its own package is
   deferred to the AOT-core cutover (piece 3), which is what actually
   shrinks `main`.

2. **Registry-triggered loading, not top-of-Load chaining.** Every non-entry
   package emits

   ```go
   func init() { rt.RegisterLib("my-app.util", Load) }
   ```

   and the requiring package links it with a blank import (`_ "<mod>/my_app/
   util"`), which is what makes the Go linker keep it. `require`'s `loadLib`
   consults the provider registry when a namespace isn't yet present, so the
   **replayed `(require …)` form itself** triggers the dependency's `Load()`
   at exactly the source position where the interpreter loads the file —
   byte-identical side-effect order by construction. `Load()` stays guarded
   by a bool (`loaded`), giving exactly-once. Cycles are rejected at
   **compile time** by the module compiler's in-progress stack (message
   mirrors Clojure's `Cyclic load dependency: a -> b -> a`); runtime replay
   therefore cannot cycle.

3. **Cross-ns var references stay interned-registry, not direct Go
   symbols.** Emitted code in A references B's var through the same hoisted
   package-level intern it uses for its own vars:

   ```go
   var v_my_app_util_offset = lang.InternVarName(lang.NewSymbol("my-app.util"), lang.NewSymbol("offset"))
   ```

   Interning is global, idempotent and order-free (design/00 §4.4), so A's
   package init and B's `Load()` land on the **same `*lang.Var` object**
   regardless of init order; the value appears when B's `Load()` runs (which
   the require replay guarantees happens first). The per-call cost is one
   atomic `v.Get()` — **identical to an intra-namespace reference** by
   construction: the emitted call site is the same `vN.Get()` load either
   way (only the strings inside the one-time package-init intern differ),
   and ADR 0004 already mandates per-call var deref for REPL/compile
   liveness parity. Measured: the CI perf budgets (`pkg/emit/perf_test.go`,
   design/00 §1.4) are unchanged by this change — cross-ns references add
   zero per-call work. A direct Go symbol (`util.V_offset`) would save
   nothing per call and would break on `binding`/redef; direct **calls**
   that bypass the Var belong to the direct-linking rung of the design/04 §5
   performance ladder, orthogonal to this change.

4. **Source resolution is requiring-file-relative.** cljgo has no classpath;
   a missed `require` of `x.y` resolves against the requiring file's source
   root: root = `dir(*file*)` minus the requiring ns's own directory suffix
   (`src/my_app/core.clj` in ns `my-app.core` → root `src/`), falling back
   to `dir(*file*)`; candidate files `<root>/x/y.clj` then `.cljg`. The same
   resolver serves the interpreter (which now loads-and-evals the file — the
   dual harness's eval half needs the identical semantics) and the module
   compiler (which loads-analyzes-evals and **captures** the forms for
   emission). A file that fails to create the required namespace is an
   error, as on the JVM.

## Consequences

- Multi-file programs compile to standalone binaries whose interpreted and
  compiled outputs are byte-identical, including load-time side-effect
  order — proven by dual-harness conformance tests and a 3-ns pkg/emit test.
- The single-file path is unchanged: `WriteProgram` with zero dependency
  namespaces delegates to the existing `WriteModule`/`EmitMain`.
- `pkg/eval` gains a lib-provider registry plus a per-evaluator `LibLoader`
  seam; emitted binaries register providers from package `init()` (no boot
  dependency — registration is a map write).
- Prerequisite for AOT-core (ADR 0023 piece 2/3): `core.clj` can now become
  an emitted package whose `Load()` replaces the interpreter's boot load;
  what remains is migrating Go builtins to `pkg/lang` and cutting
  `rt.Boot()`'s `eval.New()` edge.
- Non-goal here: `binding`-aware parallel loading, `:reload`/`:reload-all`,
  multiple source roots (a deps/paths config is a later, separate decision).
