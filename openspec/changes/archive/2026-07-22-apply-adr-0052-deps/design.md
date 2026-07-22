## Context

ADR 0052 is the authority; this is its implementation design. The §6a blocker
(third-party `go-require` REPL-vs-binary divergence) is fixed and archived (ADR
0049), so decisions 1–6 are unblocked. This is **greenfield in `pkg/`**: no
`pkg/deps` package exists, and all cache/lock/CAS/flock/git-archive machinery
lives only in `spikes/s33-.../prototype/`. Per ADR 0027 spike code never merges —
the prototypes are **re-authored** into `pkg/deps` with tests, not copied.

Verified touch-points (Explore pass, at `8d97431`):
- **`pkg/eval/libload.go:65-100`** — `ResolveLibPath`; roots built at `:75-86`,
  resolution loop `:88-98`. One non-test caller `loadLibFile` (`:37-51`), wired
  via `corelib.SetLibFileLoader` (`pkg/eval/builtins.go:49`). The three new slots
  are all-new appends to `roots` at the `:86`/`:88` seam — exactly where S30's
  `prototype.patch` inserts.
- **`pkg/build/build.go`** — `Plan` struct `:74-80` (`GoRequires []GoRequire`,
  no dep field); `LoadPlan` `:85-120`; `planFromValue` reads `:go-requires` at
  `:137-142`; `SynthGoMod` called `:264`; `go mod tidy` (the MVS delegation) at
  `goModTidy` `:314-321`, invoked `:282`. `(dep …)` is **entirely
  unimplemented** — no `dep` in `core/build.cljg`, no `:deps` key read.
- **`cmd/cljgo/main.go:50-96`** — `switch args[0]` dispatch; `runCache` attaches
  as a new `case "cache"`. (No `publish` verb today — that's ADR 0054.)
- **`pkg/reader`** — `ReadString`, `Reader.ReadAll()` parse EDN into cljgo data;
  `evalLibFile` (`libload.go:106-133`) and S32 exp4 are working templates.

## Goals / Non-Goals

**Goals:**
- A `pkg/deps` package implementing decisions 1 (cache), 3 (lock), 4/5/6
  (version conflict, transitive-from-lock, purity), re-authored from S33/S31/S32
  prototypes with tests.
- The load-path slots (decision 2) grown into `ResolveLibPath`, serving both legs
  from the one resolver.
- `(dep …)` surfaced from `build.cljgo` into the `Plan`, resolved before
  compilation, its roots handed to slot 3 and its impurity gated.
- A `cljgo cache clean` verb (decision 1).

**Non-Goals:**
- A package registry/index, publishing (ADR 0054), semver ranges / a constraint
  solver, private/authenticated sources, vulnerability scanning — all explicitly
  out of scope per ADR 0052.
- Any `pkg/` change lifted verbatim from a spike — spikes are reference; code is
  re-authored with tests (ADR 0027).
- Re-litigating §6a — done in ADR 0053.

## Decisions

1. **`pkg/deps` package, re-authored from S33.** Port `cas.go` (`CacheRoot()`
   → `$CLJGO_CACHE`→`$XDG_CACHE_HOME/cljgo`→`~/.cache/cljgo`; `TreeHash` merkle;
   `withLock`/`syscall.Flock(LOCK_EX)`; `publishAtomically`; `markReadOnly`),
   `resolve.go` (`LoadLock`/`WriteLock` for `build.lock.edn`; `resolveRef` via
   `git ls-remote`; `materialize` via `git archive` + flock + atomic rename,
   integrity-checked vs locked tree hash; `manifestDeps` transitive;
   `LoadPathRoots(deps)` in lock order), and `edn.go` (deterministic sorted-key
   EDN emitter → byte-identical lockfiles). **Replace S33's throwaway
   `ScanBuildFile` string-scanner** with real `(dep …)` reading off the evaluated
   `Plan` (below) — resolution reads the lock and manifest as data, never
   evaluates a dep build fn (decision 5).

2. **Load-path slots into `ResolveLibPath`.** At the `roots` seam
   (`libload.go:86`), append in ADR-0048 §2 order: project declared roots →
   (existing requiring-file roots, kept — "append, never replace") → resolved
   dependency roots in lock order (from `deps.LoadPathRoots`) → provider registry
   already outranks via `loadLib`. Dependency roots are threaded in via a
   process-level resolved-deps handle set by the build/run bootstrap, read by
   `ResolveLibPath` — the same handle both legs see, so parity holds by
   construction. Env `$CLJGO_PATH` augments `run` only; a build that would bake an
   env-root source **errors** (decision 2 footgun clause).

3. **`(dep …)` surface.** Add `dep` to `core/build.cljg` (a pure `swap!` onto a
   `:deps` vector, mirroring `go-require`) and its AOT mirror
   `pkg/coreaot/cljgobuild/cljgobuild.go`. Add `Deps []Dep` to `Plan`
   (`build.go:74`), populated in `planFromValue` parallel to the `:go-requires`
   loop. `buildArtifact` resolves deps (cache + lock) **before**
   `emit.CompileProgram`, hands roots to slot 3, and merges dep `:go-require`s
   into the go.mod set.

4. **Version conflict merged BEFORE the go.mod write.** cljgo detects duplicate
   Go-module requires at different versions in its own layer (port S31's merge
   cases) and hard-errors naming both requirers + both versions, **before**
   `SynthGoMod` (`build.go:264`) and before `goModTidy` (`build.go:282`) — so
   Go's silent MVS is never reached. A consumer-side override (an accepted-version
   map in `build.cljgo`) resolves it. `:requires` provenance from the lock
   (decision 3) is what makes the error message nameable.

5. **Transitive from the lock.** `manifestDeps` walks `:requires`/`:impure` as
   data; no dep build fn runs. Local `:path` deps are named holes
   (`:local/unlocked? true`).

6. **Purity gate, default-deny, ffi/cgo split.** Port S32 exp4's resolve-time
   checker: read the dep manifest with `pkg/reader`, gate on the consumer's
   acknowledged capability set (`:allow`), refuse unacknowledged `:impure` before
   fetch, refuse `:cgo` under a declared cross `:target`, and route a dep's FFI/
   go-require into the consumer go.mod (closing the ADR 0044 hole). `:ffi` and
   `:cgo` are distinct switches.

7. **`cljgo cache clean`.** New `case "cache"` → `runCache` in `main.go`; calls a
   `deps.CacheClean()` that chmods `0555` trees writable then removes them.

## Risks / Trade-offs

- **Real git in tests** → hermetic: tests create throwaway local git repos in
  `t.TempDir()` and use `file://` transports (S33 did exactly this; darwin-only
  `flock` is already how S33 ran). No network in the test suite.
- **`flock` is POSIX; Windows CI** → guard the lock behind a build-tag or a
  portable fallback; ADR 0052 §1 flagged the Windows equivalent as owed. Keep the
  POSIX path working; stub/skip on Windows with a `//go:build` split so CI stays
  green (the fundamentals batch already fixed Windows CI once — don't re-break).
- **Threading resolved-deps into `ResolveLibPath` without a global** → a
  process-scoped handle set once at bootstrap, not a package var mutated per call;
  both legs set it at the same seam so they cannot diverge. Covered by a
  dual-harness parity case (ADR 0007/0053).
- **Scope creep** → strictly decisions 1–6 + cache-clean. No registry, no
  publish, no solver. Each is ADR 0052 "out of scope".
- **`SynthGoMod` write-once is load-bearing** (ADR 0052 consequences): the
  dep-aware go.mod must still be written once, fully-formed, into a fresh temp
  `genDir`. Preserve `buildArtifact`'s `os.MkdirTemp` discipline (`build.go:225`).

## Migration Plan

Additive: no existing `build.cljgo` breaks (no `:deps` key today). A project gains
deps only by adding `(dep …)`. First resolution writes `build.lock.edn`; absent
deps, no lock is written. `cache clean` is new. Rollback = revert the change;
nothing persistent outside `~/.cache/cljgo` (removable via the new verb).

## Open Questions

- Exact `build.cljgo` surface for the **consumer-side version override** and the
  **capability `:allow` set** — S32 used `build.cljgo.policy.edn`/`:allow`; fold
  into `build.cljgo` as `(accept-version …)` / `(allow-capability …)` or a plan
  option. Resolve during task 3 authoring; keep it code, not a second manifest
  (ADR 0021).
- Windows `flock` equivalent — implement POSIX now, leave a typed TODO + skip.
