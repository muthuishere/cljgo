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

