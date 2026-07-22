## 1. `pkg/deps` foundation — lock schema + deterministic EDN (decision 3)

- [x] 1.1 Create `pkg/deps` package. Re-author S28's `edn.go` as `pkg/deps/edn_{emit,read}.go`: a deterministic sorted-key EDN emitter (byte-identical output) plus a reader built on `pkg/reader` (`ReadAll`), not a bespoke parser
- [x] 1.2 Define the `Lock`/`LockedDep` types and `LoadLock`/`WriteLock` (`build.lock.edn`): top `:lock/version`, `:build/hash`; per-dep `:name :git/url :git/ref :git/sha :tree/hash :paths :requires` and `:pure? true` | `:impure {…}`; deps name-sorted, keys sorted
- [x] 1.3 `:path` local deps recorded as named holes (`:local/unlocked? true`, unhashed)
- [x] 1.4 Lock is authoritative on `:git/sha`: a divergence check that errors naming both the lock SHA and a disagreeing ref SHA
- [x] 1.5 Tests: round-trip a lock byte-identically; two independent writes of the same graph are `cmp`-equal; divergence errors legibly; local-hole preserved

## 2. `pkg/deps` cache — identity key, content verify, flock (decision 1)

- [x] 2.1 Re-author S28's `cas.go` as `pkg/deps/cache.go`: `CacheRoot()` (`$CLJGO_CACHE` → `$XDG_CACHE_HOME/cljgo` → `~/.cache/cljgo`), `dl/` + `src/` layout
- [x] 2.2 Identity key `sha256(url‖sha‖subdir)` computable before fetch; `TreeHash(dir)` merkle content hash recomputed on every read
- [x] 2.3 `materialize`: bare git mirror into `dl/`, `git archive` into an immutable `0555` `src/` tree; `flock(LOCK_EX)` + atomic rename; losing racers discard, no temp leftovers; `flock` behind a POSIX build-tag (`lock_unix.go`) with an `O_EXCL` spin fallback (`lock_windows.go`) so Windows CI stays green
- [x] 2.4 Verify-by-content on read: recompute tree hash, error with expected/got on mismatch; a force-moved tag with unchanged sha+hash resolves unchanged
- [x] 2.5 `deps.CacheClean()`: chmod `0555` trees writable, remove cleanly
- [x] 2.6 Tests (hermetic — local `file://` repos in `t.TempDir()`, no network): warm/offline resolve with remotes removed; 13-byte tamper caught; force-moved tag no-op; 8 concurrent cold resolvers → 8× ok / 1 entry / 0 leftovers; cache-clean removes a `0555` tree

## 3. `(dep …)` surface + resolver orchestration (decisions 4, 5, 6)

- [x] 3.1 Add `dep` to `core/build.cljg` (pure `swap!` onto `:deps`) and its AOT mirror `pkg/coreaot/cljgobuild/cljgobuild.go` (regenerated via `go generate` — parity by construction); consumer-side `accept-version` / `allow-capability` verbs (code in `build.cljgo`, not a second manifest)
- [x] 3.2 Add `Deps []deps.Dep` (+ `AcceptVersions`, `AllowCaps`) to `Plan`; populate in `planFromValue` parallel to the `:go-requires` loop
- [x] 3.3 Resolver: read the lock + a dep's declarative manifest as data (`pkg/reader`), walk `:requires`/`:impure` transitively; **never evaluate a dep build fn**
- [x] 3.4 Version-conflict detection merged in the cljgo layer BEFORE the go.mod write: duplicate module at two versions hard-errors naming both requirers + both versions; consumer `accept-version` override resolves it; runs before `SynthGoMod`/`goModTidy`. Also enforced on the **no-deps self-conflict** path (`mergeOwnGoRequires`)
- [x] 3.5 Purity gate: default-deny; unacknowledged `:impure` refused before fetch naming the capability; `:ffi`/`:cgo` separate switches; `:cgo` refused under a declared cross `:target` (reachable via `ResolveOptions.CrossTargets`); a dep's FFI/go-require routed into the consumer go.mod (close ADR 0044 hole)
- [x] 3.6 `LoadPathRoots(deps)` returns resolved dependency roots in lock order; `vendor/<name>/` overrides the cache under the same lock hash
- [x] 3.7 Tests: transitive graph recovered as data with provenance, no build fn run; version conflict errors + override resolves; unacknowledged impurity refused; cgo-under-cross refused, ffi permitted

## 4. Load path — grow `ResolveLibPath` (decision 2)

- [x] 4.1 At the `roots` seam append slots in §2 order: requiring-file roots (kept) → resolved dependency roots in lock order; provider registry already outranks via `loadLib`
- [x] 4.2 Thread resolved-deps in via a process-scoped handle (`deps.SetResolvedRoots`/`ResolvedRoots`) set at the build/run bootstrap — the same handle both legs see
- [x] 4.3 `clojure.*` cannot be shadowed (provider precedence preserved); env `$CLJGO_PATH` augments `run` only (`envPathEnabled`, scoped/restored) and a build baking an env-root source errors
- [x] 4.4 Tests: a dep namespace outside the consumer tree resolves byte-identically both legs (`TestParityDependencyNamespace`); decoy/shadow/`clojure.string`-hijack covered in `pkg/deps`/`libload` tests

## 5. CLI + wiring

- [x] 5.1 `cmd/cljgo/main.go`: new `case "cache"` → `runCache`; dep resolution wired into `buildArtifact` (before compile) and the `run` bootstrap (`resolveRunDeps`), plus `cljgo dev` and `cljgo test` (`bri.go`)
- [x] 5.2 `cljgo cache clean` verified (removes a `0555` tree; bad subcommand → usage/exit 2)

## 6. Dual-harness parity + gates

- [x] 6.1 `TestParityDependencyNamespace` (conformance): a program requiring a dependency namespace resolves identically under the interpreter and the real AOT `cljgo build` binary — hermetic, resets globals
- [x] 6.2 Full gates green: `go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates && go test ./...`

## 7. Close-out

- [x] 7.1 No spike code merged verbatim into `pkg/`; prototypes stay reference-only (ADR 0027) — `pkg/deps` re-authored, verified by the adversarial review
- [x] 7.2 Update ADR 0048 status proposed → accepted (implemented); ADR 0021/0044 amendments owed recorded (not silently done)
- [ ] 7.3 `/opsx:archive` this change

## Deferred follow-ups (tracked, not blocking — outside the ratified `run`/`build` paths)

- `-update` re-pin CLI flag: today re-pinning is triggered by lock *absence*; a warm lock is authoritative. An explicit `cljgo build -update` is a small follow-up.
- `CrossTargets` is reachable via `deps.Resolve` but not yet fed from real `:target` declarations (`host-target`/`option` are bri stubs) — the `:cgo`-under-cross refusal is testable but not wired to a project cross-target yet.
- Cross-set (own vs dep) conflict message loses the dep-name provenance because `resolved.GoRequires` is flattened before the second merge — conflict is still detected; only the message degrades.
