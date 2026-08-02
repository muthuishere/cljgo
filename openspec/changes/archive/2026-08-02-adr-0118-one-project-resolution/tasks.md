# tasks — adr-0118-one-project-resolution

**Backfilled 2026-08-02 from shipped code** (`237f0b1`, `8f3fd4f`, `92fb2f5`;
v0.8.7). Boxes are checked because the work is released.

Gate: `CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates core && go test ./... -timeout 1800s -p 1`

## 1. Hoist
- [x] 1.1 `resolveProjectForCommand(args)` called from `run()` before the subcommand switch; the five per-call-site invocations deleted, not supplemented.
- [x] 1.2 Anchor on `run`'s script path, so `cljgo run /elsewhere/foo.clj` still finds /elsewhere's project.

## 2. The two lists
- [x] 2.1 `commandsThatSkipProjectResolution` — a DENY-list: `new`, `version`/`--version`/`-v`/`-version`, `help`/`--help`/`-h`, `explain`, `build`, `dist`, `publish`, `cache`, `config`, `generate`/`g`, `routes`, `migrate`. Each entry justified at the declaration.
- [x] 2.2 `build`/`dist`/`publish` are on it because they CREATE the lock this resolution reads — resolving first made `cljgo build` on a fresh clone print "no build.lock.edn … run `cljgo build` once" and then succeed, having just written it.
- [x] 2.3 `commandsThatTolerateResolutionFailure` — `repl`, `nrepl` only. Everything else exits 1 on a resolution failure.

## 3. Delete the waste (independent of the hoist)
- [x] 3.1 Dedupe `[filepath.Dir(file), "."]` — both are `"."` for the common shapes, so the whole pipeline (including an interpreter boot) ran twice. 74 ms on the `cljgo new` layout.
- [x] 3.2 `build.LoadPlan` once per pass — it ran again in the no-lock branch for a plan that was thrown away. 39.1 ms per boot.

## 4. Tests
- [x] 4.1 `cmd/cljgo/repl_resolution_test.go:TestREPLResolvesTheProjectsOwnNamespace` and `TestNREPLResolvesTheProjectsOwnNamespace` — each asserting on the call's own result rather than a trailing marker (the REPL continues past a failed require, so a marker proves nothing).
- [x] 4.2 `TestREPLAndRunAgreeOnResolution` — pins the invariant rather than the symptom: whatever `cljgo run` can require from a directory, `cljgo repl` can too.

## 5. Measurement
- [x] 5.1 `spikes/s72-project-resolution-cost/RESULTS.md` — cost by project shape and size, with allocation.
- [x] 5.2 `spikes/s78-project-resolution-crossruntime/RESULTS.md` — cljgo, let-go, Glojure and JVM Clojure in one table, with the not-apples-to-apples asymmetries stated.

## 6. Gates
- [x] 6.1 Full gate green; released in v0.8.7.
