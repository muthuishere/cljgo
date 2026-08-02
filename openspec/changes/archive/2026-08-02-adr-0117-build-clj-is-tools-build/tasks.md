# tasks — adr-0117-build-clj-is-tools-build

**Backfilled 2026-08-02 from shipped code** (`a186f25`, v0.8.6). Boxes are
checked because the work is released, not because a plan was followed.

Gate: `CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates core && go test ./... -timeout 1800s -p 1`

## 1. Narrow the probe list
- [x] 1.1 `build.BuildFileNames` = `["build.cljgo", "build.cljg"]`; `build.clj` removed, with the reason recorded at the declaration so it cannot be re-added by someone reading ADR 0055 alone.
- [x] 1.2 `FindBuildFile` unchanged in shape — first present wins, most-specific-first.
- [x] 1.3 `.clj` **source** resolution left alone (ADR 0055 decision 1 still holds).

## 2. Tests
- [x] 2.1 `pkg/build/findbuildfile_test.go` — a lone `build.clj` yields `""`, and never shadows a real cljgo build file in the dual-host layout.
- [x] 2.2 `pkg/build/consumer_defects_test.go:TestJVMBuildCljDoesNotBreakTheProject` — runs the **real cljgo binary** on koine's layout (a tools.build `build.clj` at the root plus `src/demo/app.cljc`) and asserts `cljgo run` succeeds and never mentions `clojure.tools.build.api`. Confirmed to fail with the reported error when the entry is restored, so the test proves the fix rather than merely accompanying it.

## 3. Gates
- [x] 3.1 Full gate green; released in v0.8.6.
