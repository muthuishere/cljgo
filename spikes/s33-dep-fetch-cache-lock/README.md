# Spike S33 — What is the smallest fetch + cache + lock that makes `(dep b …)` reproducible?

Opened 2026-07-22. Feeds **ADR 0048** decisions **1** (cache) and **3**
(lockfile). Runs in parallel with S30 (load path), S31 (version conflict),
S32 (purity/manifest).

## Context

ADR 0021 decision 3 gives cljgo-package deps the surface
`(dep b "name" {:git … | :path …})` — "a Zig `build.zig.zon` analog, but
expressed in the same file". That settled the surface and none of the
mechanics.

ADR 0048's verified current state is blunt: **the cljgo-library lane does
not exist.** `ResolveLibPath` (`pkg/eval/libload.go:65`) resolves only
relative to `dir(*file*)`. There is no load path, no dependency root, no
cache, no lock. A fetched dependency has nowhere to be put.

S30 owns *where a resolved dep is found from* (the load path). **S33 owns
what makes the thing at that path trustworthy and identical everywhere.**
The division: S30 answers "how does `a.core` resolve"; S33 answers "is the
`a.core` you resolved the same bytes I resolved, on a different machine,
with a cold cache, offline, and can we prove it if someone tampers".

Two constraints from ADR 0048 shape the lock schema before any code:

- **Decision 5 forbids executing a dep's `build` fn at resolve time.** A
  dep's transitive requirements therefore cannot be discovered by running
  it. Either they are in the lock, or they are nowhere. **The lock is where
  transitivity must live.**
- **Decision 6 (purity) is UNRESOLVED**, but names `go-require` / `c-link` /
  `ffi` propagation as the open question. Whatever S32 concludes, resolution
  can only act on impurity at resolve time if impurity is *recorded* at
  resolve time. So the lock schema must reserve room for impurity markers
  even though S33 does not decide policy on them.

## The one question

**What is the smallest fetch + cache + lock mechanism that makes
`(dep b "name" {:git … | :path …})` reproducible — and what, exactly, must
the lockfile contain?**

## Exit criterion (written before any code, per ADR 0027)

A prototype resolver (throwaway self-contained Go module in this directory)
against **local bare git repos as fixture remotes** — no network dependency
on github for the core proof. The criterion is met iff all five hold, each
backed by captured real command output in `results/`:

1. **Cross-machine determinism.** Two clean-cache resolutions of the same
   `build.cljgo`, run in **different directories with different cache
   roots** (the machine proxy), produce **byte-identical dependency trees**,
   verified by a content hash over the materialized tree — and the two
   generated `build.lock.edn` files are byte-identical.
2. **Tamper detection.** Mutating one byte inside a warm cache entry is
   **detected on the next resolve** and fails with a legible error naming
   the dep, the expected hash, and the got hash. A silent success closes the
   spike **no**.
3. **Moved-ref detection.** A tag force-moved (and a branch advanced) in the
   fixture remote **does not** change what a locked build resolves; and an
   *unlocked* re-resolve that would pick up the new commit is reported as a
   lock/remote divergence rather than silently applied.
4. **Offline.** With the lock present and the cache warm, resolution
   completes with **no network and no access to the fixture remote at all**
   (the bare repo is renamed away for the duration). Any attempt to reach a
   remote is a failure of this criterion.
5. **Concurrency safety** (ADR 0048 §1's explicit ask). N concurrent
   resolvers against one shared cold cache produce one correct cache entry,
   no partial/corrupt entries, no error — proven by running N processes and
   hashing the result.

Anything less closes S33 **no** for the mechanism tested, and ADR 0048
decision 1 or 3 changes accordingly.

## What must additionally be investigated and reported

1. **Cache layout.** Exact path. The repo already has user-level state
   precedent — `pkg/repl/session.go:57` hardcodes
   `~/.config/cljgo/sessions`, **not** XDG-aware. ADR 0048 §1 proposes
   `$XDG_CACHE_HOME/cljgo/` with `~/.cache/cljgo/` fallback. Report the
   inconsistency and recommend. Content-addressed **by what** — the git SHA,
   or a hash of tree contents? Compare against Go's module cache
   (`$GOMODCACHE`, `lock`/`.info`/`.ziphash`, `GONOSUMDB`/`go.sum`) and say
   what each choice buys.
2. **Vendor escape hatch.** Project-local `vendor/` for air-gapped builds.
   Does it interact cleanly with S30's load path ordering (ADR 0048 §2), or
   does it need its own precedence slot?
3. **Lockfile schema.** Propose it, prototype it, and argue every field —
   including which candidate fields were considered and **rejected**.
4. **`:path` deps.** They need no fetching and are not reproducible across
   machines. Do they belong in the lock at all? Recommend.
5. **Failure legibility.** Every failure mode above must produce an error a
   human can act on without reading the resolver's source.

## Deliberately NOT built (per ADR 0048 "out of scope")

No registry, no index, no publishing, no semver ranges, no constraint
solver, no authenticated/private sources, no vulnerability scanning. Version
*conflict* policy is S31's, not S33's. Purity *policy* is S32's; S33 only
proves the lock can carry the markers.

## Method

Throwaway Go module in this directory (`prototype/`). Spike code **never**
merges into `pkg/` (ADR 0027 §5). `pkg/`, `cmd/`, `core/`, `templates/` are
not touched. Fixture remotes are local bare git repos created by
`prototype/fixture.sh`. Binaries are gitignored (`spikes/**/*.bin`). All
claims in `VERDICT.md` are backed by real captured output in `results/`.

## Results

See `VERDICT.md`. Raw data in `results/`.
