# tasks — adr-0119-deps-edn-paths

**Backfilled 2026-08-02 from shipped code** (`c2bfa04`, v0.8.7). Boxes are
checked because the work is released.

Gate: `CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates core && go test ./... -timeout 1800s -p 1`

## 1. Read the file
- [x] 1.1 `pkg/deps/depsedn.go` — `DepsEDNFileName`, `DepsEDNPaths(dir) []string`, parsed with the existing EDN reader (`edn_read.go`), never the evaluator.
- [x] 1.2 `:paths` only, vector-of-strings only; relative entries resolved against `dir`; non-directories dropped.
- [x] 1.3 Every error path returns nil — no diagnostic, no failure to start.

## 2. Precedence, in two passes
- [x] 2.1 `resolveRunDeps` checks EVERY search directory for a cljgo build file first; only when none exists anywhere is `deps.edn` consulted. The first cut used a per-directory fallback and read a decoy `deps.edn` beside the script while a `build.cljgo` sat in the working directory.
- [x] 2.2 `addSourceRoots` shared by the build-file and `deps.edn` paths, so the two cannot drift in how they publish roots (append, never replace; duplicates skipped).

## 3. Tests
- [x] 3.1 `cmd/cljgo/repl_resolution_test.go:TestREPLResolvesADepsEdnOnlyProject` — koine's shape: `deps.edn` with `:paths`, no build file.
- [x] 3.2 `TestBuildCljgoWinsOverDepsEdn` — `deps.edn` points at a decoy directory; the cljgo build file wins, so the precedence cannot silently invert.
- [x] 3.3 `TestBuildCljgoWinsFromAnyDirectoryInTheSearch` — decoy `deps.edn` beside the script, `build.cljgo` in the working directory: the exact shape the per-directory fallback got wrong. Confirmed to fail (reading the decoy) when the two-pass order is reverted.
- [x] 3.4 `cmd/cljgo/depsedncost_bench_test.go` — the read+parse+stat cost measured through the production path (`resolveRunDeps`), not a toy.

## 4. Verification against the real consumer
- [x] 4.1 Manual, against koine 0.9.0: `run-conformance.sh` green on both hosts before and after (13 checks, 271 assertions, no regression), and `cljgo repl` at the project root now resolves `koine.json` — it failed on v0.8.6 and on ADR 0118's branch.

## 5. Gates
- [x] 5.1 Full gate green; released in v0.8.7.
