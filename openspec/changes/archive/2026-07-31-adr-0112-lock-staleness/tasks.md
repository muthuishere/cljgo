# tasks — adr-0112-lock-staleness

## 1. Staleness detection
- [x] 1.1 `deps.DeclaredSetHash([]Dep) string` — sorted `(name, version-or-ref, kind)` triples, hashed. Bytes of `build.cljgo` are NOT an input.
- [x] 1.2 Write it into the lock on every rewrite; keep carrying it on re-resolve.
- [x] 1.3 `resolveDeps`: `update := lock == nil || lock.BuildHash != hash`.
- [x] 1.4 Test: cosmetic manifest edit (comment, artifact rename) does not re-resolve.

## 2. Minimal re-resolve
- [x] 2.1 Carry unchanged pins into the new resolution as pre-seeded lock entries.
- [x] 2.2 Re-resolve only changed declarations and what is reachable only from them.
- [x] 2.3 Test: bumping root `a` leaves every pin reachable only from `b` byte-identical.

## 3. Frozen mode
- [x] 3.1 `ResolveOptions.Frozen`; stale under Frozen is a coded error naming both versions, and MUST NOT write the lock.
- [x] 3.2 `cljgo build --locked` + `CLJGO_LOCKED=1`.
- [x] 3.3 Register the diagnostic + `docs/diagnostics/<CODE>.md`; regenerate the registry lock.
- [x] 3.4 Test: frozen + stale fails and leaves the lock byte-identical; frozen + matching succeeds online with no re-resolution.

## 4. Remove the dead end
- [x] 4.1 Delete the note naming `resolve -update`; grep for other diagnostics naming nonexistent commands.
- [x] 4.2 Update `site/.../guides/deps-publish.md`.

## 5. Gates
- [x] 5.1 Full gate green; `CLJGO_CLOJARS_IT=1` deps integration test green.
