## ADDED Requirements

### Requirement: cljgo's core is AOT-compiled and linked, not interpreted
Every embedded boot source in `core.BootSources()` SHALL be compiled by
`pkg/emit` into a Go package under `pkg/coreaot`, whose guarded `Load()`
runs that source's top-level forms under the same `*ns*`/`*file*` frame
the interpreter's loader pushes. `pkg/coreaot.Load()` SHALL run them in
table order and intern the `user` namespace, and `rt.Boot()` SHALL
bootstrap a binary with `corelib.RegisterAll()` + that loader, never by
constructing an Evaluator.

#### Scenario: a compiled binary links no interpreter
- **WHEN** `go list -deps` runs on `pkg/coreaot` and `pkg/emit/rt`
- **THEN** the closure contains none of pkg/eval, pkg/analyzer,
  pkg/ast, pkg/emit, pkg/repl

#### Scenario: the generated core cannot drift from its sources
- **WHEN** `cmd/gencore` is re-run into a temp dir (a Go test)
- **THEN** its output is byte-identical to the committed `pkg/coreaot`

#### Scenario: one table drives both modes
- **WHEN** the interpreter boots (`eval.New`) and when the core is
  generated (`cmd/gencore`)
- **THEN** both walk `core.BootSources()` in the same order, loading
  the same namespaces

### Requirement: regex literals compile as constants
A `#"…"` literal SHALL emit as a package-level `&reader.Regex` value,
one per literal site, never deduplicated across sites.

#### Scenario: Pattern identity semantics survive compilation
- **WHEN** a compiled program evaluates two separately-read `#"same"`
  literals
- **THEN** they are not `=` to each other, matching the interpreter and
  JVM Clojure's Pattern
