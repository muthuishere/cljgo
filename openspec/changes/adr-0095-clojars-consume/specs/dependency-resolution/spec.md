## ADDED Requirements

### Requirement: A Maven/Clojars coordinate is one more shape on the one resolver

`build.cljgo` MUST accept `(dep b "group/artifact" {:mvn/version "V"})` as a
third dependency shape beside `{:git …}` and `{:path …}`. A single-segment name
MUST be read as `group == artifact`. Resolution MUST use the Go standard library
only — no JVM, no `mvn`, no shelled tool. The resolved source MUST be mounted on
the existing ADR 0052 load path (slot 3) as an ordinary source root, so
`pkg/emit` is unchanged and the REPL and AOT legs resolve identically. Declaring
`:mvn/version` together with `:git` or `:path` MUST raise `G5015`.
`(mvn-repo b url)` MUST prepend to the default repository list, which MUST be
Clojars then Maven Central.

#### Scenario: a pure library resolves and its namespace is requirable

- GIVEN a fixture repository serving `org.clojure/tools.cli 1.1.230`
- WHEN a project declares `(dep b "org.clojure/tools.cli" {:mvn/version "1.1.230"})` and resolves
- THEN the extracted source root is on the load path and `(require '[clojure.tools.cli])` succeeds
- AND the same program produces byte-identical output under `cljgo run` and a compiled binary

#### Scenario: conflicting coordinate keys are refused

- GIVEN `(dep b "x/y" {:mvn/version "1.0" :git "https://…"})`
- WHEN the project resolves
- THEN `G5015` is raised naming the dep and both coordinate keys, and nothing is fetched

### Requirement: transitive .pom resolution covers a named slice and name-errors the rest

The resolver MUST walk `<dependencies>` breadth-first, honouring `<scope>`
(skipping `test`/`provided`/`system`/`import`), `<optional>`, and `<exclusions>`
including `*` wildcards. `org.clojure/clojure`, `org.clojure/spec.alpha` and
`org.clojure/core.specs.alpha` MUST be pruned from every graph and the prune
MUST be reported, not silent. A repeated `group/artifact` at a differing version
MUST raise `G5013` naming both versions and both requirers, unless
`accept-version` pins it. `${property}` interpolation, `<dependencyManagement>`
version supply, `<parent>` inheritance, version ranges, `-SNAPSHOT`,
`<profiles>`, `<classifier>` and non-jar `<packaging>` MUST raise `G5011`
naming the feature and the coordinate — they MUST NOT be partially supported and
MUST NOT resolve to a guessed version.

#### Scenario: an uninterpolated property version is named, not guessed

- GIVEN a pom whose dependency version is `${clojure.version}` on a non-pruned coordinate
- WHEN the project resolves
- THEN `G5011` names the coordinate, the element, and the unsupported feature
- AND no request for a literal `${clojure.version}` artifact is made

#### Scenario: a version conflict names both requirers

- GIVEN two dependencies that transitively require `a/b` at `1.0` and `2.0`
- WHEN the project resolves
- THEN `G5013` names `a/b`, both versions, and both requiring coordinates, and suggests `accept-version`

### Requirement: Maven artifacts are cached, locked and verified like git deps

The jar and pom bytes MUST be cached under `CacheRoot()` and the extracted
source published atomically and read-only. `build.lock.edn` MUST record
`:mvn/group`, `:mvn/artifact`, `:mvn/version`, `:mvn/repo`, `:mvn/sha256`,
`:mvn/pom-sha256`, `:tree/hash` and `:paths`. The extracted tree MUST be
re-hashed on every read and a mismatch MUST raise `G5012` naming expected,
found, and the cache path. A repo-served `.sha1` MUST be verified when present.
Extraction MUST drop `.class` files, `META-INF/`, absolute and `..` zip paths,
and MUST bound entry count and size.

#### Scenario: offline resolution from a warm cache touches no network

- GIVEN a locked Maven dep whose extracted tree is in the cache
- WHEN the project resolves with `-offline`
- THEN resolution succeeds with no HTTP request and the tree hash is verified

#### Scenario: offline with a cold cache is a named error

- GIVEN a locked Maven dep absent from the cache
- WHEN the project resolves with `-offline`
- THEN `G5014` names the coordinate and the cache path checked, and suggests dropping `-offline`

### Requirement: the Java gate is per-namespace, loud on use, never a whole-library ban

A Maven dependency's namespaces MUST be classified individually. Namespaces free
of Java interop MUST load and be usable even when the same artifact also ships
Java-tainted namespaces. Requiring a Java-tainted namespace MUST raise `I4002`
naming the namespace, the coordinate, the `file:line` of the offending form, and
the number of usable namespaces in the same artifact. It MUST NOT return `nil`,
MUST NOT be a whole-library refusal, and MUST behave identically under `cljgo
run` and in a compiled binary. Classification MUST run on reader output (after
reader conditionals are resolved), MUST NOT flag bare `(.method obj)`, class-ref
values, `instance?` or `catch` forms, and MUST be recorded per namespace in the
lock so `cljgo resolve` reports usable-vs-Java counts offline. A dependency
whose namespaces are all Java-tainted MUST resolve with a loud warning, not a
failure.

#### Scenario: a mixed artifact yields its pure namespaces

- GIVEN an artifact shipping 8 pure and 2 Java-importing namespaces
- WHEN a project requires one of the 8
- THEN it loads and works
- AND `cljgo resolve` reports "8 namespaces usable, 2 require Java" naming the two

#### Scenario: requiring the Java namespace fails loud with a location

- GIVEN the same artifact
- WHEN a project requires one of the 2 Java namespaces
- THEN `I4002` names the namespace, the coordinate, and the `file:line` of the `(:import …)` form
- AND the identical message appears from the REPL, `cljgo run`, and the compiled binary

### Requirement: reader conditionals resolve by the reader, and a starved conditional fails loud

`.cljc` sources from a Maven dependency MUST be read by `pkg/reader` with the
platform features `:cljgo` and `:default` and MUST NOT be classified by text
scanning. Java fenced inside a non-selected branch MUST NOT taint the namespace.
For maven-origin files only, a `#?`/`#?@` form with at least one branch of which
none is selectable MUST raise `R1012` naming the file, line, column, the
expected features and the features found. Default reading semantics for project
code MUST be unchanged. A `.cljc` whose entire body is unselectable MUST fail
loud at require and MUST NOT install an empty namespace.

#### Scenario: Java fenced in a :clj branch leaves the namespace pure

- GIVEN a `.cljc` whose only Java use is inside `#?(:clj … :default …)`
- WHEN it is classified and required
- THEN it is pure, it loads, and the `:default` branch's value is used

#### Scenario: a :clj/:cljs-only conditional is named, not silently empty

- GIVEN a maven-origin `.cljc` containing `#?(:clj a :cljs b)`
- WHEN it is read
- THEN `R1012` names the file:line:col, Expected `:cljgo, :default`, Found `:clj, :cljs`
- AND no namespace with zero vars is installed

#### Scenario: text that merely looks like a conditional is not one

- GIVEN a file containing `"#?(:clj x)"` in a string, after `;`, and after `#_`
- WHEN it is read and classified
- THEN no conditional is resolved from any of them and no `R1012` is raised

### Requirement: dependency-resolution errors are registered diagnostics, and tests never touch the network

Every user-facing failure on the Maven path MUST carry a registered code from
`pkg/diag/registry.go` with a `docs/diagnostics/<CODE>.md` explain page, MUST
state Expected vs Found where the shape is expected-vs-actual, MUST carry
suggestions as `Fix` values rather than prose, and MUST render identically in
the REPL, `cljgo run`, compiled binaries and the `--json` envelope. No raw Go
panic, transport error, or bare `fmt.Errorf` string may reach a user on this
path. No committed test may perform a network request; Maven behaviour MUST be
tested against an in-process `httptest` repository with test-built fixture jars,
guarded by a transport that fails any off-fixture request.

#### Scenario: a missing coordinate names every repo tried

- GIVEN a coordinate absent from both configured repositories
- WHEN the project resolves
- THEN `G5010` names the coordinate, each repo URL, and each HTTP status, and offers a did-you-mean Fix when a near name is already locked

#### Scenario: a transport failure is a diagnostic, not a stack trace

- GIVEN a repository that closes the connection mid-response
- WHEN the project resolves
- THEN a registered diagnostic is rendered and no Go panic, goroutine dump, or `*url.Error` text reaches the user
