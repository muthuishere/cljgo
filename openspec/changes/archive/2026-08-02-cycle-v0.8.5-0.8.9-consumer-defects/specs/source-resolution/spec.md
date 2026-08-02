## ADDED Requirements

### Requirement: one source-extension list, read by every subsystem that needs it

The accepted source-file extensions MUST exist as a single shared list
(`pkg/eval.SourceExts`, most-specific-first `.cljgo` > `.cljg` > `.clj` >
`.cljc`). No subsystem may carry its own copy.

Deriving a namespace symbol from a file path MUST strip whichever accepted
extension is present, using that same list. A second, hand-maintained copy is
forbidden: two subsystems disagreeing about what a file's namespace is called
produces a require for a name that cannot exist, and the disagreement is
invisible until the extension that only one copy knows about is used.

#### Scenario: a `.cljc` suite runs through the compiled leg

- GIVEN a test namespace in a file `test/demo/core_test.cljc`
- WHEN `cljgo test --compiled` runs the suite
- THEN the derived namespace is `demo.core-test`, not `demo.core-test.cljc`
- AND the suite runs and reports, rather than failing to locate a namespace

#### Scenario: both legs agree on a `.cljc` suite

- GIVEN the same suite
- WHEN it is run interpreted and compiled
- THEN both legs run the same tests and agree on the result — a green
  interpreted leg MUST NOT coexist with a compiled leg that cannot build
