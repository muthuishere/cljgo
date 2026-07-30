# Tasks — adr-0105-cljx-test

Gate: `CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates core && go test ./... -timeout 1800s -p 1`
Never hand-edit generated briaot files — run `go generate ./pkg/briaot`.

## 1. cljx.test core namespace
- [x] 1.1 `core/cljx/test.cljg` — satellite preamble; mock/calls/call-count/called?/called-with?; spy/stub/with-spies over with-redefs; capturing + printed/printed?; expect matcher family over clojure.test `is`; describe/it over deftest/testing; before-each/after-each/before-all/after-all over use-fixtures. Plus with-frozen-time/advance! over cljg.date.
- [x] 1.2 Registry scoping — solved by REMOVING the registry. s66's fn-keyed
  registry is unsound in compiled mode: a compiled fn has no stable identity
  ((= f1 f2) is TRUE for distinct closures, (identical? f1 f1) is FALSE), so
  neither `=` nor `identical?` can key it. Took s66 finding 2's stated
  alternative — the sentinel-arg protocol (`:cljx.test/call-log`): each mock
  closes over its own log, so there is no global state to leak or clear.
- [x] 1.3 Embed in `core/cljx.go`; Specs() row LAST (`cljx.test`, Pkg `cljxtest`, no shim); `go generate ./pkg/briaot`.
- [x] 1.4 Output capture wired as the DEFAULT for cljx tests (not opt-in), with the installed clojure.test reporter routed PAST the capture buffer so FAIL lines are not swallowed, and the captured output replayed under a failing test.
- [x] 1.5 Conformance `conformance/tests/cljx-test-*.clj` — mock-recording (+ no-leak), spy-stub (forward/replace/restore-on-throw), capture (+ stub compose), expect-matchers, expect-failure-message (+ capture replay), hooks-and-time. All six dual harness (eval + compiled, byte-identical).

## 2. Runner correctness
- [x] 2.1 Compiled test binaries exit non-zero on failure (QA bug: currently exit 0 — CI green on red).
      `clojure.test/-process-failures` (a process-level tally kept in `do-report`) + the emitted
      `func main()`'s `cljgoTestsFailed()` check; `cljgo run` does the same so the legs agree on exit status.
- [x] 2.2 `cljgo test --compiled` runs the suite through the AOT path. `--both` landed too: it runs the
      interpreted leg in a child process, the compiled leg in-process, and diffs output + exit code.
- [x] 2.3 Failure report: drop the `#=(var ...)` leak, include file:line of the failing assertion.
      Now `FAIL in (failing-on-purpose) (test/mylib/core_test.cljg:18)` — names from `*testing-vars*`,
      position from the reader's metadata on the `is` form (`*assertion-position*`).

## 3. fn metadata (conformance gap, s66)
- [x] 3.1 Make `(with-meta (fn [] 1) {..})` work like JVM Clojure, OR freeze the divergence in conformance with a documented rationale. → **implemented** (option a): `lang.MetaFn` boxes a closure with its map; `FnFuncN`/`NamedFnN`/`*eval.evalFn` all carry metadata now, both legs, no hot-path cost.
- [x] 3.2 Conformance test citing the JVM oracle either way. → `conformance/tests/fn-metadata.clj` + `fn-metadata-invoke.clj`, dual harness.

## 4. Adoption
- [ ] 4.1 Template tests (lib/cli/web) use cljx.test idioms.
- [ ] 4.2 ADR 0085 forward-pointer: the `cljx.*` developer-experience tier exists.
- [ ] 4.3 Close-out: full gate green; archive this change.
