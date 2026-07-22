# Spike S25 — Can a load path make out-of-tree libraries resolve identically in both legs?

Opened 2026-07-21. Feeds **ADR 0048**. Follows ADR 0042 (multi-namespace
emission), whose §4 is the resolution rule this spike proposes to widen.

## Context — the verified current state

- `pkg/eval/libload.go:65` `ResolveLibPath` resolves a namespace symbol
  **only** relative to `dir(*file*)`: candidate roots are `dir(*file*)`
  stripped of the requiring ns's own directory suffix (so
  `src/my_app/core.clj` in ns `my-app.core` yields root `src/`), then
  `dir(*file*)` itself; candidate files `<root>/<munged stem>.clj` then
  `.cljg`. **No load path of any kind exists.** ADR 0042's own
  non-goals list says as much: "multiple source roots (a deps/paths
  config is a later, separate decision)".
- `pkg/emit/module.go` builds the AOT module by **evaluating** require
  forms through the interpreter: `CompileProgram` installs
  `moduleCompiler.load` as `ev.LibLoader`, and `pkg/eval`'s
  `loadLibFile` calls `ResolveLibPath` and hands the resolved path to
  that loader (`libload.go:37-51`). The emitter has **no independent
  resolver**.

The second bullet is the central de-risk claim of the whole ADR 0048
design: *one resolver change serves both legs*. It is stated here as a
hypothesis to be **verified by measurement**, not assumed.

## The one question

**Can a load path be added to namespace resolution such that a cljgo
library whose source lives OUTSIDE the consuming project's source tree
resolves identically under both legs — `cljgo run` (interpreter) and
`cljgo build` (AOT binary)?**

## Exit criterion (written before any code, per ADR 0027)

A fixture with two disjoint directories:

- `libsrc/a/core.clj` — namespace `a.core`, itself requiring a sibling
  `a.util` from **its own** root (the recursion trap), and printing at
  load time (so side-effect ORDER is observable).
- `app/src/main.clj` — an entry namespace outside `libsrc/` that
  `(require 'a.core)`.

The criterion is met iff, with the prototype resolver in place and
`libsrc/` on the load path:

1. `cljgo run app/src/main.clj` succeeds and prints output X.
2. `cljgo build app/src/main.clj` succeeds, and the generated Go module
   contains a package directory for `a.core` (`a/core/core.go`) **and**
   for `a.util`.
3. The built binary prints stdout **byte-identical to X** (`cmp` on
   captured files, exit 0) — including load-time side-effect order.

Anything less — e.g. the run leg works but the emitter cannot see the
foreign namespace, or the two stdouts differ — closes this spike **no**,
and ADR 0048 must then design two resolvers or a different mechanism.

## What must additionally be investigated and reported

1. **Roots and precedence** — where roots come from (project source
   roots → declared deps → embedded core) and in what order they are
   consulted.
2. **Ambiguity** — same namespace available from two roots: first-wins,
   error, or shadow? What does JVM Clojure's classpath do (oracle
   against the real `clojure` CLI, never memory)?
3. **Does the emitter really inherit the fix**, or does it have hidden
   resolution assumptions? Embedded/core namespaces are handled
   specially (provider registry) — check exactly where that branch is
   taken and whether a load-path hit can be mistaken for one.
4. **`*file*` / `*ns*` semantics across roots** — `evalLibFile` pushes
   `VarFile` to the dep's path. Does a dep's own relative sibling
   require then resolve from **its** root rather than the consumer's?
   This is the likely trap and is deliberately baked into the fixture.
5. **`$CLJGO_PATH`-style env override** — worth having, or a footgun
   (irreproducible builds, machine-dependent binaries)?

## Method

Throwaway prototype in this directory. Spike code never merges into
`pkg/` (ADR 0027). Where proving the resolver genuinely requires
patching `pkg/eval/libload.go`, the patch is applied as a **local
experiment, measured, then reverted**, and frozen here as
`prototype.patch`; the tracked tree is left clean. All claims in
`VERDICT.md` must be backed by real captured command output.

## Results

See `VERDICT.md`.
