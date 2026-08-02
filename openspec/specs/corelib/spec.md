# corelib Specification

## Purpose
TBD - created by archiving change builtins-to-lang. Update Purpose after archive.
## Requirements
### Requirement: interpreter-independent builtins package
The Go-native clojure.core builtins that do not require evaluator state
SHALL live in `pkg/corelib`, registered into clojure.core via
`corelib.RegisterAll()` without constructing an Evaluator. The package
SHALL NOT depend on `pkg/eval`, `pkg/analyzer`, `pkg/ast`, or
`pkg/emit`, directly or transitively.

#### Scenario: import hygiene is machine-checked
- **WHEN** `go list -deps github.com/muthuishere/cljgo/pkg/corelib`
  runs in CI (a Go test)
- **THEN** the dependency closure contains none of pkg/eval,
  pkg/analyzer, pkg/ast, pkg/emit

#### Scenario: interpreter behavior is unchanged
- **WHEN** the conformance dual harness and the jank clojure-test-suite
  run after the move
- **THEN** every frozen `;; expect:` output matches in BOTH modes and
  the suite scoreboard equals the pre-change baseline

### Requirement: evaluator-coupled builtins stay in pkg/eval
Builtins whose semantics require the interpreter — `macroexpand-1`,
`macroexpand`, `eval`, `require-go` — SHALL be registered by `pkg/eval`
on evaluator construction, through the same `corelib.Def` seam.
`require` SHALL NOT be among them: its libspec surface and the
lib-provider registry live in `pkg/corelib`, and only its source-file
loading is supplied by the interpreter through
`corelib.SetLibFileLoader`.

`pkg/corelib` SHALL additionally register an AOT definition for each of
the four, which `pkg/eval` overwrites when an Evaluator is constructed,
so that a binary with no interpreter behaves predictably rather than
hitting unbound vars.

#### Scenario: interpreted behavior is unchanged
- **WHEN** an Evaluator is constructed (REPL, `cljgo run`, the
  conformance eval harness)
- **THEN** `eval`, `macroexpand`, `macroexpand-1` and `require-go`
  behave exactly as before this change

#### Scenario: eval in an AOT binary fails honestly
- **WHEN** an AOT-compiled binary calls `(eval …)`, `(macroexpand …)`
  or `(macroexpand-1 …)`
- **THEN** it throws an error stating the name is not available in an
  AOT-compiled binary because no analyzer is linked
- **AND** the var is bound, so `resolve` / `bound?` / `#'eval` as a
  value behave as in the REPL

#### Scenario: require-go in an AOT binary is a no-op
- **WHEN** an AOT-compiled binary replays `(require-go '[… :as …])`
- **THEN** it succeeds and does nothing — the emitter already resolved
  and linked those Go calls at compile time

#### Scenario: macro engine still reachable from the REPL
- **WHEN** `(macroexpand-1 '(when true 1))` runs in the REPL
- **THEN** it expands exactly as before the move

### Requirement: require works without an interpreter
`require` SHALL resolve a namespace from the lib-provider registry or an
already-present namespace in ANY binary, applying `:as` / `:refer` /
prefix-list options identically in both modes. When neither yields the
namespace and no source-file loader is installed, it SHALL fail with an
error naming the AOT limitation.

#### Scenario: registry-triggered dependency load in a binary
- **WHEN** a compiled binary replays a `(require 'my-app.util)` whose
  package registered a provider from init()
- **THEN** the dependency's Load() runs at that source position, once

#### Scenario: a filesystem-backed require in a binary
- **WHEN** a compiled binary requires a namespace with no provider and
  no existing namespace
- **THEN** the error states that this is an AOT-compiled binary with no
  interpreter to load the namespace from source

### Requirement: the interop and exception substrate is interpreter-free
The reflect-backed Go interop path (`CallGoMethod`, `GoFieldGet`,
`GoFieldSet`, `MakeGoStruct`, `NewGoStruct` and the result-shaping
table) and the exception normalizers (`Throw`, `Recover`,
`CatchMatches`) SHALL live in `pkg/corelib` and be the SINGLE
implementation both modes call.

#### Scenario: byte-identity is preserved by construction
- **WHEN** the interpreter evaluates a dot-form / throw / try and an
  AOT binary runs the emitted equivalent
- **THEN** both call the same corelib function and produce identical
  output

