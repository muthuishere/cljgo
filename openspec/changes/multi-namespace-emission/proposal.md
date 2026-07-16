# multi-namespace-emission

## Why

ADR 0023's structural fix — AOT-compiling core.clj so emitted binaries drop
the interpreter (~2MB, ~2ms startup) — requires the emitter to produce more
than one namespace. design/04 §7 sketched this as v0.5 ("multiple namespaces
+ `:require`, per-package `Load()` chaining"); nothing implements it. Today
`pkg/emit` compiles exactly one `.clj` file into one `package main`, and
`require` panics for any namespace that isn't embedded at boot. This change
is piece 1 of the AOT-core milestone: multi-namespace emission with
correctly-chained loading. Pieces 2 (builtins → pkg/lang) and 3 (core
cutover) build on it and are explicitly out of scope.

Relies on: **ADR 0042** (accepted, oracled 2026-07-17 against Clojure
1.12.5) — one Go package per ns, registry-triggered loading, interned
cross-ns var refs, requiring-file-relative source resolution. Contract owner:
design/04 §1 (namespace → package, `Load()`, bootstrap `main`) as amended by
ADR 0042; design/00 §4.4 (interning is idempotent/order-free) is what makes
cross-package interns sound.

## What Changes

- **pkg/eval** — `require`'s `loadLib` gains two seams when a namespace is
  missing: a **lib-provider registry** (`RegisterLibProvider`; emitted
  packages register `Load` from `init()`) and a **filesystem loader** using
  ADR 0042's requiring-file-relative resolution. The interpreter's default
  loader reads-and-evals the file (dual-harness eval half); compile-time
  cycle detection with Clojure-style `Cyclic load dependency` errors.
- **pkg/emit** — new module compiler `CompileProgram` (entry file + its
  file-backed requires, captured per namespace, dependency order) and
  `WriteProgram` (one Go package per dependency namespace + the existing
  main package for the entry). `EmitMain`/`WriteModule`/`CompileFile` keep
  their signatures and single-file behavior byte-for-byte; `WriteProgram`
  with zero dependency namespaces delegates to `WriteModule`.
- **pkg/emit/rt** — `RegisterLib(name, load)` re-export for emitted code.
- **cmd/cljgo build / pkg/build** — `Build` and `buildArtifact` compile via
  `CompileProgram`/`WriteProgram` so `cljgo build` handles multi-file
  programs.
- **conformance** — `compiledOutput` moves to `CompileProgram`/`WriteProgram`
  (identical path for every existing single-ns test); new multi-ns
  conformance tests (dep files under `tests/multi/`, outside the harness
  glob) run in BOTH harnesses; a 3-ns compile-and-run proof lands in
  `pkg/emit`.

## Non-goals

- Pieces 2/3 of AOT-core (builtin migration, `rt.Boot` cutover, core.clj as
  an emitted package).
- `:reload` / `:reload-all`, parallel/thread-safe loading beyond a mutex on
  the provider registry, multiple source roots or a deps/paths config.
- Splitting the ENTRY namespace out of `package main` (deferred to piece 3,
  where `main` actually shrinks).
- Direct Go symbol references across namespaces (design/04 §5 ladder rung 1;
  ADR 0042 records why they buy nothing today).

## Impact

- Affected specs: `emitter` (new capability spec).
- Affected code: `pkg/eval/builtins.go` (+ new `libload.go`),
  `pkg/emit/{compile,program,emit}.go` (+ new `module.go`), `pkg/emit/rt`,
  `pkg/build/build.go`, `conformance/compiled_test.go`, new conformance
  files. `pkg/lang` untouched.
- Risk: REPL-vs-binary divergence in load order — mitigated by the
  registry-triggered design (ADR 0042 §2) and dual-harness tests.
