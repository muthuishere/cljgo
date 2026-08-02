# Tasks — apply-adr-0068-cljc-and-repl-cwd-resolution

## 1. Resolver

- [x] 1.1 Add `.cljc` (last) to the extension probe in `ResolveLibPath`
      (`pkg/eval/libload.go`)
- [x] 1.2 No-file contexts (`""`/`NO_SOURCE_FILE`/`NO_SOURCE_PATH`/`REPL`)
      root at `.` instead of returning `""`; dependency roots + `$CLJGO_PATH`
      unchanged after
- [x] 1.3 Update the file's doc comment + not-found error to mention `.cljc`

## 2. CLI

- [x] 2.1 `isSourceFile` accepts `.clj`/`.cljc`/`.cljg`/`.cljgo` via
      `filepath.Ext` (`cmd/cljgo/main.go`)
- [x] 2.2 `defaultBinaryName` strips whichever accepted extension is present

## 3. Conformance

- [x] 3.1 Fixture `conformance/tests/conf/hostdemo.cljc` with
      `:clj`/`:cljs`/`:cljgo`/`:default` branches
- [x] 3.2 Test `conformance/tests/cljc-require.clj` requiring the fixture,
      `;; expect:` frozen from the `:cljgo` selections; JVM oracle verified
      2026-07-23 (`["jvm" "jvm-only" "x@jvm"]` — hosts select their own
      branches by design, ADR 0036)
- [x] 3.3 Dual harness green (REPL + AOT) — TestConformanceEval/cljc-require
      + TestConformanceCompiled/cljc-require both PASS

## 4. Gates

- [x] 4.1 `go build ./... && go vet ./... && gofmt -l pkg cmd conformance
      templates && go test ./...` all green (2026-07-23)
