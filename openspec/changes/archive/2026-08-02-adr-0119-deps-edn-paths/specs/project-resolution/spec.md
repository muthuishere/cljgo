## ADDED Requirements

### Requirement: a project with no cljgo build file gets its source roots from `deps.edn` `:paths`

When no cljgo build file exists anywhere in the resolution search, cljgo MUST
read `:paths` from the directory's `deps.edn` and publish those directories as
source roots.

The reading MUST be the narrowest one that can be exactly right:

- `:paths` and nothing else — not `:deps`, `:aliases`, `:extra-paths` or
  `:replace-paths`, which carry tools.deps alias semantics cljgo does not
  implement.
- Only the vector-of-strings form. The map form MUST be skipped, not guessed
  at.
- Parsed as DATA with the EDN reader. Reading `deps.edn` MUST NOT boot the
  evaluator.
- Every error MUST be swallowed to "no roots learned". A malformed or exotic
  `deps.edn` belongs to the JVM toolchain and MUST NOT prevent cljgo from
  starting. This mechanism can only ADD roots the project itself declared, so
  it can never make resolution resolve the wrong file.

#### Scenario: a dual-host library's REPL sees the project

- GIVEN a project with `deps.edn` declaring `:paths ["src"]`, a tools.build
  `build.clj`, and no cljgo build file
- WHEN `cljgo repl` is started at the project root
- THEN the project's own namespaces under `src/` can be required

#### Scenario: an unreadable deps.edn is not fatal

- GIVEN a project whose `deps.edn` cannot be parsed, or whose `:paths` is a map
- WHEN cljgo resolves the project
- THEN no roots are learned from it and cljgo starts normally

### Requirement: a cljgo build file wins absolutely over `deps.edn`

Where a `build.cljgo` or `build.cljg` exists ANYWHERE in the resolution search,
`deps.edn` MUST NOT be consulted at all — not merged, not consulted for keys
the build file omits, not used as a fallback for anything.

Resolution MUST therefore make TWO passes: every search directory is checked
for a cljgo build file first, and only if none exists anywhere is `deps.edn`
considered. A per-directory fallback is forbidden, because it lets a
`deps.edn` next to the script win over a `build.cljgo` in the working
directory.

#### Scenario: the build file beats a deps.edn in another search directory

- GIVEN a script in a directory whose `deps.edn` points at a decoy source root
- AND a `build.cljgo` in the working directory declaring the real roots
- WHEN `cljgo run <that script>` resolves the project
- THEN the roots come from `build.cljgo` and the decoy is never read
