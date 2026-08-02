# source-resolution Specification

## Purpose
How a namespace symbol resolves to a source file: the accepted extension set
and its precedence, the candidate roots, and the rule that every subsystem
deriving a namespace from a path reads the same list.
## Requirements
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

### Requirement: Namespace source resolution accepts four extensions, most-specific-first

`ResolveLibPath` MUST probe, in order, `.cljgo`, `.cljg`, `.clj`, `.cljc`
for each candidate root, returning the first existing file. `.cljc` is the
portable multi-host fallback and MUST rank last, mirroring the JVM's
`.clj` > `.cljc` preference. Both execution legs MUST inherit this from the
single shared resolver.

#### Scenario: A .cljc library namespace loads via require
- **WHEN** `lib.cljc` exists in a candidate root and no `lib.cljgo`/`lib.cljg`/`lib.clj` does, and a program evaluates `(require 'lib)`
- **THEN** `lib.cljc` loads, with reader conditionals selecting the `:cljgo`/`:default` branches (ADR 0036), identically in the REPL, `cljgo run`, and an AOT-compiled binary

#### Scenario: More-specific extension wins over .cljc
- **WHEN** both `lib.clj` and `lib.cljc` exist in the same root
- **THEN** `lib.clj` is loaded

### Requirement: No requiring-file context roots resolution at the process cwd

When `*file*` is unset (`""`, `NO_SOURCE_FILE`, `NO_SOURCE_PATH`, or
`REPL`), `ResolveLibPath` MUST use the process cwd (`.`) as the
requiring-file root instead of failing. Dependency roots (ADR 0052 §2) and
`$CLJGO_PATH` MUST still append after, unchanged in order; the provider
registry still outranks all roots.

#### Scenario: REPL requires a cwd namespace
- **WHEN** the interactive REPL evaluates `(require 'demo)` and `demo.cljc` (or any accepted extension) exists in the cwd
- **THEN** the namespace loads and its vars are accessible

### Requirement: The CLI recognizes all four extensions as source-file arguments

`cljgo build <arg>` MUST treat an argument ending in `.clj`, `.cljc`,
`.cljg`, or `.cljgo` as a source file (not a build-step name), and the
default binary name MUST be the file's base name with that extension
stripped (`core` still resolves to its parent directory's name).

#### Scenario: Building a .cljc file names the binary correctly
- **WHEN** `cljgo build demo.cljc` runs with no `-o`
- **THEN** the produced binary is named `demo` (plus the platform exe suffix), not `demo.cljc`

