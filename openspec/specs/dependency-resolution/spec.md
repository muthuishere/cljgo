# dependency-resolution Specification

## Purpose
TBD - created by archiving change apply-adr-0052-deps. Update Purpose after archive.
## Requirements
### Requirement: Fetched dependencies live in a global cache keyed by identity and verified by content

Fetched dependencies MUST be stored in a global cache at `$XDG_CACHE_HOME/cljgo/`
(falling back to `~/.cache/cljgo/`), with `dl/` holding bare git mirrors and
`src/` holding immutable `0555` source trees materialized by `git archive`. A
cache slot MUST be located by an **identity key** `sha256(url‖sha‖subdir)`
computable before the fetch, and every read MUST **verify content** by
recomputing a merkle tree hash — a git SHA alone MUST NOT be treated as a content
guarantee.

#### Scenario: Warm cache resolves offline with remotes removed
- **WHEN** a dependency is already materialized in `src/` and its lock entry is
  present, and the git remotes are unavailable
- **THEN** the build resolves it from cache without any network access

#### Scenario: A tampered cache entry is detected
- **WHEN** the bytes of a materialized `src/` tree differ from the lock's
  `:tree/hash`
- **THEN** resolution fails with an error stating the expected and actual hash,
  rather than using the tampered tree

#### Scenario: A force-moved tag does not change a locked build
- **WHEN** a git tag is moved to a different commit but the lock's `:git/sha` and
  `:tree/hash` are unchanged
- **THEN** the locked build resolves the originally-pinned content

#### Scenario: Concurrent cold-cache resolvers are safe
- **WHEN** multiple resolvers materialize the same dependency into a cold cache
  concurrently
- **THEN** each exits successfully, exactly one immutable entry results, and no
  temporary directories are left behind (`flock` + atomic rename, losing racers
  discard their work)

#### Scenario: cache clean removes immutable trees
- **WHEN** the user runs `cljgo cache clean`
- **THEN** the `0555` immutable trees are removed cleanly (a plain `rm -rf`
  cannot)

### Requirement: A project-local vendor directory overrides the cache

A project-local `vendor/<name>/` MUST override the cache for that dependency when
present, under the same lock hash, without introducing a new load-path slot.

#### Scenario: vendored source wins over cache
- **WHEN** `vendor/<name>/` exists for a locked dependency
- **THEN** the resolver uses the vendored tree instead of the cache entry, and
  the build is otherwise identical

### Requirement: The load path serves both legs from one resolver

`ResolveLibPath` MUST resolve a required namespace against, in order: (1) the
requiring file's own roots (appended to, never replaced), (2) the project's
declared source roots, (3) resolved dependency roots in lock order (the slot a
`vendor/<name>/` override varies), (4) provider/registered namespaces. The same
resolver MUST serve the interpreter and the emitter's namespace-discovery pass.

#### Scenario: A dependency outside the consumer tree resolves identically in both legs
- **WHEN** a program requires a namespace supplied only by a resolved dependency
  root
- **THEN** `cljgo run` and the `cljgo build` binary resolve it to the same source
  (byte-identical), with the AOT module containing a package for that namespace

#### Scenario: A decoy in the consumer root does not shadow a dependency namespace
- **WHEN** the consumer's own root contains a file whose name collides with a
  dependency namespace, and the dependency file resolves from its own root
- **THEN** the dependency's own file wins (each file resolves from its own root
  because `*file*` is rebound), because roots are appended, never replaced

### Requirement: Provider namespaces outrank all roots

Provider/registered and already-present namespaces MUST be consulted before
`ResolveLibPath`, so a root carrying `clojure/string.clj` (or any `clojure.*`)
is ignored. `clojure.*` MUST NOT be shadowable by a dependency or project root.

#### Scenario: A root cannot hijack clojure.string
- **WHEN** a dependency or project root contains `clojure/string.clj`
- **THEN** the built-in `clojure.string` is used and the root's file is ignored

### Requirement: Environment-supplied roots may not feed a build artifact

A `$CLJGO_PATH`-style environment root MAY augment `cljgo run`, but MUST NOT
contribute source to a `cljgo build` artifact, so the same command cannot produce
a different binary per machine.

#### Scenario: Env root is refused for build
- **WHEN** a `cljgo build` would incorporate source reachable only via an
  environment-supplied root
- **THEN** the build errors (or excludes it) rather than silently baking
  machine-specific source into the binary

### Requirement: A committed lockfile pins every dependency

`build.lock.edn` (EDN, adjacent to `build.cljgo`, committed) MUST record per
dependency: `:name`; `:git/url`, `:git/ref` (provenance), `:git/sha` (identity);
`:tree/hash`; `:paths`; `:requires` (transitive dependency names); and
`:pure? true` or `:impure {…}`. Top level MUST carry `:lock/version` and
`:build/hash`. Dependencies MUST be name-sorted and map keys sorted, so the file
is byte-identical across machines. The lock MUST be authoritative on `:git/sha`.

#### Scenario: A build.cljgo ref disagreeing with the lock is an error
- **WHEN** a `build.cljgo` `(dep …)` ref resolves to a `:git/sha` different from
  the lock's
- **THEN** resolution errors naming both the lock's SHA and the ref's SHA, and
  MUST NOT silently re-pin

#### Scenario: Local path deps are recorded as named holes
- **WHEN** a dependency is declared with `:path` (local)
- **THEN** the lock records it with `:local/unlocked? true`, unhashed, preserving
  its load-path position and transitive deps without pretending to pin it

#### Scenario: The lockfile is byte-identical across machines
- **WHEN** the same resolved graph is locked on two machines
- **THEN** the two `build.lock.edn` files are byte-identical (name-sorted deps,
  sorted map keys)

### Requirement: Version conflicts hard-error, detected before the go.mod write

A duplicate Go-module require at two different versions MUST be detected by cljgo
and hard-error **before** the `go.mod` is written — never delegated to
`go mod tidy`, which silently applies MVS (exit 0, higher version wins). The
error MUST name both requirers and both versions. A consumer-side override MUST
let the consumer accept a specific version. cljgo MUST NOT run a version solver.

#### Scenario: Two deps pinning different versions of one module error
- **WHEN** dep A requires `go-cmp v0.6.0` and dep B requires `v0.7.0`
- **THEN** the build hard-errors naming A, B, and both versions — it MUST NOT
  silently link `v0.7.0`

#### Scenario: A consumer override resolves the conflict
- **WHEN** the consumer declares an explicit accepted version for the conflicting
  module
- **THEN** the build proceeds with that version and no error

### Requirement: Transitive dependencies come from the lock, never from a dep's build fn

Resolution MUST read transitive requirements from the lock's `:requires`/`:impure`
data and a dependency's declarative manifest surface only. It MUST NOT evaluate a
dependency's `(defn build …)`.

#### Scenario: Transitive graph recovered without evaluating any build fn
- **WHEN** the resolver walks a multi-level dependency graph
- **THEN** it recovers every transitive require with its requirer provenance from
  lock data, evaluating no dependency build function

### Requirement: Dependency impurity is default-deny with explicit capability opt-in

An impure dependency (carrying `go-require`, `ffi`, or `cgo`/`c-link`) MUST
resolve only if the consumer explicitly acknowledges that capability.
Unacknowledged impurity MUST be refused, not warned. `:ffi` and `:cgo` MUST be
separate switches. `:cgo` MUST be refused (not warned) when the project declares
cross-compilation targets. A dependency's `:go-require`s MUST merge at the cljgo
layer (subject to the version-conflict rule), not via `go mod tidy`.

#### Scenario: Unacknowledged impure dependency is refused
- **WHEN** a dependency declares `:impure` and the consumer has not acknowledged
  that capability
- **THEN** resolution refuses the dependency before fetching, naming the
  capability that must be acknowledged

#### Scenario: cgo is refused under cross-targets
- **WHEN** the project declares a cross-compilation `:target` and a dependency
  requires `:cgo`
- **THEN** resolution refuses it, distinct from an `:ffi` dependency which is
  permitted

#### Scenario: A dependency's FFI requirement reaches the consumer go.mod
- **WHEN** a pure-Clojure consumer depends on a dependency that declares an FFI/
  Go-module requirement
- **THEN** that requirement is included in the consumer's build so the binary
  links it (closing the ADR 0044 library-carries-FFI hole)

### Requirement: a moved declaration makes the lock stale, and the lock refreshes itself

The lock MUST record a hash of the **normalised declared dependency set** —
the sorted set of `(name, version-or-ref, kind)` triples `build.cljgo`
declares, and nothing else. Resolution MUST treat the lock as stale when that
hash differs from the manifest's, and MUST re-resolve and rewrite the lock
rather than erroring.

The hash MUST NOT cover the manifest's bytes. Reformatting `build.cljgo`,
adding a comment, or renaming an artifact declares nothing new and MUST NOT
trigger a re-resolve.

No diagnostic may name a command that does not exist.

#### Scenario: bumping a version re-pins without deleting the lock

- GIVEN a project locked at `medley/medley 1.4.0`
- WHEN `build.cljgo` is changed to `{:mvn/version "1.5.0"}` and the project builds
- THEN the build succeeds and `build.lock.edn` pins `1.5.0`
- AND the user is never told to run a subcommand that does not exist

#### Scenario: a cosmetic manifest edit does not re-resolve

- GIVEN a locked project whose lock hash matches
- WHEN a comment is added to `build.cljgo` and the project builds
- THEN the lock is unchanged and no coordinate is re-fetched

### Requirement: a re-resolve moves only what changed

When the declared set moves, resolution MUST keep the existing pin of every
coordinate whose declaration is unchanged, and MUST re-resolve only the
changed declarations and the coordinates reachable only from them.

This is a correctness requirement, not an optimisation: a full re-resolve on a
one-line version bump silently drifts every other transitive to whatever is
newest today, which defeats the lock's purpose.

#### Scenario: bumping one root leaves unrelated pins untouched

- GIVEN a lock pinning roots `a`, `b` and their disjoint transitives
- WHEN only `a`'s declared version changes and the project builds
- THEN `a` and its transitives are re-resolved
- AND every pin reachable only from `b` is byte-identical to before

### Requirement: frozen mode makes a stale lock an error

`cljgo build --locked`, and the environment variable `CLJGO_LOCKED=1`, MUST
make a stale lock a hard error naming the divergent coordinates, and MUST NOT
re-resolve or rewrite the lock.

Frozen is distinct from offline: a build MAY be online and still require the
lock to be the authority. A merge that takes `build.cljgo` from one branch and
`build.lock.edn` from another MUST fail under `--locked` rather than silently
resolving a graph nobody reviewed.

#### Scenario: CI refuses a lock that does not match the manifest

- GIVEN a project whose manifest declares `1.5.0` and whose lock pins `1.4.0`
- WHEN it is built with `--locked`
- THEN the build fails with a coded diagnostic naming both versions
- AND `build.lock.edn` is unchanged on disk

#### Scenario: frozen mode is satisfied by a matching lock

- GIVEN a project whose lock hash matches its manifest
- WHEN it is built with `--locked` while online
- THEN the build succeeds and performs no coordinate re-resolution

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

### Requirement: A Maven/Clojars coordinate is one more shape on the one resolver

`build.cljgo` MUST accept `(dep b "group/artifact" {:mvn/version "V"})` as a
third dependency shape beside `{:git …}` and `{:path …}`. A single-segment name
MUST be read as `group == artifact`. Resolution MUST use the Go standard library
only — no JVM, no `mvn`, no shelled tool. The resolved source MUST be mounted on
the existing ADR 0052 load path (slot 3) as an ordinary source root, so
`pkg/emit` is unchanged and the REPL and AOT legs resolve identically. Declaring
`:mvn/version` together with `:git` or `:path` MUST raise `G5015`.
`(mvn-repo b url)` MUST prepend to the default repository list, which MUST be
Maven Central then Clojars — the order `tools.deps` itself returns
(`pkg/deps/mvncoord.go` `DefaultMvnRepos`). (This clause originally read
"Clojars then Maven Central"; the shipped order is the tools.deps one, already
frozen as its own requirement in this capability, and the delta is corrected
here rather than merged as a contradiction.)

#### Scenario: a pure library resolves and its namespace is requirable

- GIVEN a fixture repository serving `org.clojure/tools.cli 1.1.230`
- WHEN a project declares `(dep b "org.clojure/tools.cli" {:mvn/version "1.1.230"})` and resolves
- THEN the extracted source root is on the load path and `(require '[clojure.tools.cli])` succeeds
- AND both legs read the same resolved roots handle, so they cannot resolve different source

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
MUST NOT be a whole-library refusal, and MUST fire from the ONE shared loader
so the interpreter leg and the AOT leg cannot diverge — the emitter discovers
namespaces by evaluating requires, so a Java namespace fails at BUILD time and
can never be emitted into a binary. Classification MUST run on reader output (after
reader conditionals are resolved), MUST NOT flag bare `(.method obj)`, class-ref
values, `instance?` or `catch` forms, and MUST be recorded per namespace in the
lock so the usable-vs-Java counts are readable offline; resolution MUST print
that report. A dependency
whose namespaces are all Java-tainted MUST resolve with a loud warning, not a
failure.

#### Scenario: a mixed artifact yields its pure namespaces

- GIVEN an artifact shipping 8 pure and 2 Java-importing namespaces
- WHEN a project requires one of the 8
- THEN it loads and works
- AND resolution reports "8 namespace(s) usable, 2 require Java" naming the two

#### Scenario: requiring the Java namespace fails loud with a location

- GIVEN the same artifact
- WHEN a project requires one of the 2 Java namespaces
- THEN `I4002` names the namespace, the coordinate, and the `file:line` of the `(:import …)` form
- AND the same loader raises it in both legs, so a build fails rather than emitting the namespace

### Requirement: reader conditionals resolve by the reader, and a starved conditional fails loud

`.cljc` sources from a Maven dependency MUST be read by `pkg/reader` with the
platform features `:cljgo` and `:default` and MUST NOT be classified by text
scanning. Java fenced inside a non-selected branch MUST NOT taint the namespace.
For maven-origin files only, a **top-level** `#?` form with at least one branch
of which none is selectable MUST raise `R1012` naming the file, line, column,
the expected features and the features found. A conditional NESTED inside
another form MUST NOT raise it: nesting is the portable-library fencing idiom
(`#?(:clj (java.util.Date.) :default (now))`, `(:import #?(:clj …))`) and
erroring there would reject the libraries this capability exists to consume.
The consequence is a deliberate false NEGATIVE (a starved conditional nested
inside a selected branch elides silently); a false positive is the worse
failure. Default reading semantics for project
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

