# source-resolution Specification (delta)

## ADDED Requirements

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
