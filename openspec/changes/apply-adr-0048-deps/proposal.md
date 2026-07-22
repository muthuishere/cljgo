## Why

`build.cljgo` can declare a `(dep …)` (ADR 0021 surface), but a fetched
dependency has **nowhere to be put and no way to announce itself to the
resolver** — `ResolveLibPath` resolves only relative to the requiring file.
There is no load path, no cache, no lockfile, and no policy for impure
(Go/FFI/cgo) dependencies. ADR 0048 decides the mechanics; its §6a blocker
(third-party `go-require` silently diverging REPL from binary) is now fixed and
archived (ADR 0049), so decisions 1–6 are unblocked.

## What Changes

- **Global dependency cache** (`$XDG_CACHE_HOME/cljgo/`, fallback
  `~/.cache/cljgo/`): `dl/` bare git mirrors + `src/` immutable `0555` trees
  materialized by `git archive`. **Keyed by identity** (`sha256(url‖sha‖subdir)`,
  computable before fetch) and **verified by content** (a merkle tree hash
  recomputed on every read — a git SHA is not a content hash). `flock` + atomic
  rename, immutable entries. A project-local `vendor/<name>/` overrides the cache
  under the same lock hash.
- **Load path** grown into `ResolveLibPath` (the codebase's only resolver, so
  both legs inherit it): requiring-file roots (appended, never replaced) →
  project declared roots → resolved dependency roots (lock order) →
  provider/registered namespaces. **Providers outrank all roots** — `clojure.*`
  cannot be shadowed (deliberate divergence from JVM, recorded). Env-supplied
  roots are **barred from feeding a build artifact**.
- **`build.lock.edn`** — committed EDN lockfile adjacent to `build.cljgo`:
  per-dep `:name`, `:git/url`, `:git/ref`, `:git/sha`, `:tree/hash`, `:paths`,
  `:requires`, and `:pure? true` **or** `:impure {…}`. Lock is authoritative on
  `:git/sha`; a `build.cljgo` ref that disagrees is a divergence **error naming
  both**. `:path` deps stay as named holes (`:local/unlocked? true`).
- **Version selection**: explicit pins, **hard error on conflict**, actively
  detected and **merged at the cljgo layer BEFORE the `go.mod` write** — never
  delegated to `go mod tidy` (which silently applies MVS, exit 0). Error names
  both requirers and both versions; a consumer-side override is provided. No
  solver at the cljgo layer.
- **Transitive deps come from the lock, never from executing a dep's build fn** —
  `:requires`/`:impure` carry the graph as data.
- **Dependency purity — capability sets, default deny**: an impure dep resolves
  only if the consumer acknowledges it. `:ffi` (purego) and `:cgo` (`c-link`)
  are **separate switches**; `:cgo` is **refused** (not warned) when the project
  declares cross-targets.
- **`cljgo cache clean`** verb — required because `0555` trees can't be
  `rm -rf`'d cleanly.

## Capabilities

### New Capabilities
- `dependency-resolution`: how a declared `(dep …)` is fetched, cached
  (identity-keyed / content-verified), pinned (`build.lock.edn`), placed on the
  load path (both legs, one resolver), version-selected (explicit pins, hard
  error on conflict), read transitively from the lock, and gated for purity
  (capability sets, default deny; ffi/cgo split) — plus the `cljgo cache clean`
  verb.

### Modified Capabilities
<!-- No existing OpenSpec capability's requirements change. host-resolution-parity
     (ADR 0049) is a prerequisite, already satisfied, not modified here. -->

## Impact

- `pkg/eval/libload.go` (`ResolveLibPath` load-path slots).
- New `pkg/deps` package: cache (git mirror/archive, sha256 identity key, merkle
  tree hash, flock, atomic rename), `build.lock.edn` read/write, resolver
  orchestration (version conflict, transitive-from-lock, purity gate).
- `pkg/build/build.go` (`(dep …)` surfaces into the plan; version-conflict merge
  BEFORE `SynthGoMod`/`go mod tidy`; dep `go-require`/ffi reach the consumer
  `go.mod`).
- `cmd/cljgo/main.go` (`cache` subcommand).
- Conformance: dependency-loading cases run in the ADR 0007 dual harness (one
  resolver → parity free by construction).
- Frozen references adopted: S28 (cache + lock schema), S26 (lock reader), S27
  (purity validator), S25 (load-path patch).
