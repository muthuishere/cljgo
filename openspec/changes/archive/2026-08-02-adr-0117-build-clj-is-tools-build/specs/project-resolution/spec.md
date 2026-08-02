## ADDED Requirements

### Requirement: the accepted cljgo build-file names are cljgo's own, and `build.clj` is not one of them

The accepted build-description names MUST be `build.cljgo` then `build.cljg`,
most-specific-first (ADR 0055, narrowed by ADR 0117). `build.clj` MUST NOT be
probed, read, or evaluated by cljgo on any path — neither `cljgo build` nor
the project resolution that precedes every evaluating subcommand.

`build.clj` is the tools.build convention and the way a dual-host library
publishes to Clojars, so a project targeting both JVM Clojure and cljgo
necessarily has one. A directory containing only a `build.clj` MUST resolve as
"no cljgo build file".

This narrows the build-description probe only. `.clj` acceptance for **source**
resolution (ADR 0055 decision 1) is unchanged.

#### Scenario: a tools.build project runs under cljgo

- GIVEN a project with a root `build.clj` that requires
  `clojure.tools.build.api`, and a source file `src/demo/app.cljc`
- WHEN `cljgo run src/demo/app.cljc` is executed
- THEN it succeeds
- AND the output never mentions `clojure.tools.build.api`

#### Scenario: a cljgo build file is not shadowed in the dual-host layout

- GIVEN a project with both `build.clj` (tools.build) and `build.cljgo`
- WHEN cljgo locates the project's build file
- THEN it finds `build.cljgo`, and each tool reads its own file
