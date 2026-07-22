## 1. `pkg/deps` foundation — lock schema + deterministic EDN (decision 3)

- [ ] 1.1 Create `pkg/deps` package. Re-author S28's `edn.go` as `pkg/deps/edn.go`: a deterministic sorted-key EDN emitter (byte-identical output) plus a reader built on `pkg/reader` (`ReadAll`), not a bespoke parser
- [ ] 1.2 Define the `Lock`/`LockedDep` types and `LoadLock`/`WriteLock` (`build.lock.edn`): top `:lock/version`, `:build/hash`; per-dep `:name :git/url :git/ref :git/sha :tree/hash :paths :requires` and `:pure? true` | `:impure {…}`; deps name-sorted, keys sorted
- [ ] 1.3 `:path` local deps recorded as named holes (`:local/unlocked? true`, unhashed)
- [ ] 1.4 Lock is authoritative on `:git/sha`: a divergence check that errors naming both the lock SHA and a disagreeing ref SHA
- [ ] 1.5 Tests: round-trip a lock byte-identically; two independent writes of the same graph are `cmp`-equal; divergence errors legibly; local-hole preserved

## 2. `pkg/deps` cache — identity key, content verify, flock (decision 1)

- [ ] 2.1 Re-author S28's `cas.go` as `pkg/deps/cache.go`: `CacheRoot()` (`$CLJGO_CACHE` → `$XDG_CACHE_HOME/cljgo` → `~/.cache/cljgo`), `dl/` + `src/` layout
- [ ] 2.2 Identity key `sha256(url‖sha‖subdir)` computable before fetch; `TreeHash(dir)` merkle content hash recomputed on every read
- [ ] 2.3 `materialize`: bare git mirror into `dl/`, `git archive` into an immutable `0555` `src/` tree; `flock(LOCK_EX)` + atomic rename; losing racers discard, no temp leftovers; guard `flock` behind a POSIX build-tag with a Windows skip/stub (do not re-break Windows CI)
- [ ] 2.4 Verify-by-content on read: recompute tree hash, error with expected/got on mismatch; a force-moved tag with unchanged sha+hash resolves unchanged
- [ ] 2.5 `deps.CacheClean()`: chmod `0555` trees writable, remove cleanly
- [ ] 2.6 Tests (hermetic — local `file://` repos in `t.TempDir()`, no network): warm/offline resolve with remotes removed; 13-byte tamper caught; force-moved tag no-op; 8 concurrent cold resolvers → 8× ok / 1 entry / 0 leftovers; cache-clean removes a `0555` tree

## 3. `(dep …)` surface + resolver orchestration (decisions 4, 5, 6)

- [ ] 3.1 Add `dep` to `core/build.cljg` (pure `swap!` onto `:deps`) and its AOT mirror `pkg/coreaot/cljgobuild/cljgobuild.go`; add consumer-side `accept-version` / capability-`allow` surface (code in `build.cljgo`, not a second manifest)
- [ ] 3.2 Add `Deps []Dep` to `Plan` (`pkg/build/build.go:74`); populate in `planFromValue` parallel to the `:go-requires` loop (`:137`)
- [ ] 3.3 Resolver: read the lock + a dep's declarative manifest as data (`pkg/reader`), walk `:requires`/`:impure` transitively (port S26 `manifestDeps`); **never evaluate a dep build fn**
- [ ] 3.4 Version-conflict detection merged in the cljgo layer BEFORE the go.mod write: duplicate module at two versions hard-errors naming both requirers + both versions (port S26 merge cases); consumer `accept-version` override resolves it; runs before `SynthGoMod` (`build.go:264`) and `goModTidy` (`build.go:282`)
- [ ] 3.5 Purity gate (port S27 exp4): default-deny; unacknowledged `:impure` refused before fetch naming the capability; `:ffi`/`:cgo` separate switches; `:cgo` refused under a declared cross `:target`; a dep's FFI/go-require routed into the consumer go.mod (close ADR 0044 hole)
- [ ] 3.6 `LoadPathRoots(deps)` returns resolved dependency roots in lock order (port S28); `vendor/<name>/` overrides the cache under the same lock hash
- [ ] 3.7 Tests: transitive graph recovered as data with provenance, no build fn run; version conflict errors + override resolves; unacknowledged impurity refused; cgo-under-cross refused, ffi permitted

## 4. Load path — grow `ResolveLibPath` (decision 2)

- [ ] 4.1 At the `roots` seam (`pkg/eval/libload.go:86`) append slots in §2 order: project declared roots → (existing requiring-file roots, kept) → resolved dependency roots in lock order → (provider registry already outranks via `loadLib`)
- [ ] 4.2 Thread resolved-deps in via a process-scoped handle set once at the build/run bootstrap and read by `ResolveLibPath` — the same handle both legs see (no per-call global mutation)
- [ ] 4.3 `clojure.*` cannot be shadowed (provider precedence preserved); env `$CLJGO_PATH` augments `run` only and a build baking an env-root source errors
- [ ] 4.4 Tests: a dep namespace outside the consumer tree resolves byte-identically both legs; a decoy in the consumer root does not shadow the dep; a root cannot hijack `clojure.string`; env-root refused for build

## 5. CLI + wiring

- [ ] 5.1 `cmd/cljgo/main.go`: new `case "cache"` → `runCache(args[1:])` handling `clean` (and a `--help`); wire dep resolution into `buildArtifact` before `emit.CompileProgram`, and into the `run` bootstrap
- [ ] 5.2 `cljgo cache clean` end-to-end test

## 6. Dual-harness parity + gates

- [ ] 6.1 Add a conformance case (ADR 0007 dual harness): a program requiring a dependency namespace resolves identically under `cljgo run` and the `cljgo build` binary (parity free via the one resolver)
- [ ] 6.2 Full gates green: `go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates && go test ./...`

## 7. Close-out

- [ ] 7.1 Verify no spike code merged verbatim into `pkg/`; prototypes stay reference-only (ADR 0027)
- [ ] 7.2 Update ADR 0048 status proposed → accepted (implemented); note ADR 0021/0044 amendments owed are recorded (not silently done)
- [ ] 7.3 `/opsx:archive` this change
