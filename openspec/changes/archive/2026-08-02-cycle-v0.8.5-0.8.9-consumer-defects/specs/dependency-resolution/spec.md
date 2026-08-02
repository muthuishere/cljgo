## ADDED Requirements

### Requirement: the default Maven repository order matches tools.deps

The default repository set MUST be Maven Central first, then Clojars — the
order tools.deps applies unconditionally (0.22.1492,
`clojure/tools/deps/util/maven.clj:161-165`). The order MUST be pinned by a
test carrying that citation, so it cannot drift back on someone's recollection.

cljgo fetches from the first repository that answers, so a coordinate published
to both would otherwise resolve to a DIFFERENT artifact on cljgo than on the
JVM — silently, with no conflict, no diagnostic, and nothing in the project
files to hint at it. For a dual-host `.cljc` project this defeats the whole
promise below the level anyone would think to look.

A project MUST still be able to override the default set by prepending its own
repository with `(mvn-repo …)`.

#### Scenario: a coordinate on both Central and Clojars resolves as it does on the JVM

- GIVEN a Maven coordinate published to both Maven Central and Clojars
- WHEN cljgo resolves it with the default repository set
- THEN it fetches the Maven Central artifact, matching what tools.deps would
  resolve on the JVM

#### Scenario: a project can still prefer its own repository

- GIVEN a project declaring a repository with `(mvn-repo …)`
- WHEN a coordinate is resolved
- THEN the declared repository is consulted before the defaults
