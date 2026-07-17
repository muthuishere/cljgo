## ADDED Requirements

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
`macroexpand`, `eval`, `require` (file loading via LibLoader),
`require-go` — SHALL remain registered by `pkg/eval` on evaluator
construction, layered on the same `corelib.Def` seam.

#### Scenario: macro engine still reachable from the REPL
- **WHEN** `(macroexpand-1 '(when true 1))` runs in the REPL
- **THEN** it expands exactly as before the move
