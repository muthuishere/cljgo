# Tasks — adr-0105-cljx-test

Gate: `CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates core && go test ./... -timeout 1800s -p 1`
Never hand-edit generated briaot files — run `go generate ./pkg/briaot`.

## 1. cljx.test core namespace
- [ ] 1.1 `core/cljx/test.cljg` — satellite preamble; mock/calls/call-count/called?/called-with?; spy/stub/with-spies over with-redefs; capturing + printed/printed?; expect matcher family over clojure.test `is`; describe/it over deftest/testing; before-each/after-each/before-all/after-all over use-fixtures.
- [ ] 1.2 Registry scoping: per-test, cleared between tests (no global leak) — see spike s66 finding 2.
- [ ] 1.3 Embed in `core/cljx.go`; Specs() row LAST (`cljx.test`, Pkg `cljxtest`, no shim); `go generate ./pkg/briaot`.
- [ ] 1.4 Output capture wired as the DEFAULT for cljx tests (not opt-in).
- [ ] 1.5 Conformance `conformance/tests/cljx-test-*.clj`: mock recording, spy forward+restore, stub, capture (+compose), expect failure message shape. Dual harness.

## 2. Runner correctness
- [ ] 2.1 Compiled test binaries exit non-zero on failure (QA bug: currently exit 0 — CI green on red).
- [ ] 2.2 `cljgo test --compiled` runs the suite through the AOT path.
- [ ] 2.3 Failure report: drop the `#=(var ...)` leak, include file:line of the failing assertion.

## 3. fn metadata (conformance gap, s66)
- [ ] 3.1 Make `(with-meta (fn [] 1) {..})` work like JVM Clojure, OR freeze the divergence in conformance with a documented rationale.
- [ ] 3.2 Conformance test citing the JVM oracle either way.

## 4. Adoption
- [ ] 4.1 Template tests (lib/cli/web) use cljx.test idioms.
- [ ] 4.2 ADR 0085 forward-pointer: the `cljx.*` developer-experience tier exists.
- [ ] 4.3 Close-out: full gate green; archive this change.
