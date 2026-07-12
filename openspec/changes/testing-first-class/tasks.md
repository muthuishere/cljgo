## 1. Interpreted runner (can land before M2)

- [ ] 1.1 core/test.clj: `deftest`, `is` (= and thrown? cases), `testing`, report maps, counters; conformance/tests/*.clj with expectations frozen against real JVM clojure.test 1.12.5 output (oracle cited in comments). Gates: `go build ./... && go vet ./... && gofmt -l pkg cmd conformance && go test ./...` green.
- [ ] 1.2 core/test.clj: `use-fixtures` (:each/:once), `run-tests`, `run-all-tests`, `successful?`, `*test-out*`; oracle-verified conformance for fixture ordering. Gates: build/vet/gofmt/test green.
- [ ] 1.3 cmd/cljgo: `test` verb — path resolution per design D1 (project file :paths/:test-paths, defaults, fallback), load + collect `:test` vars, run, exit codes; `--ns`/`--var` filters; runner-behavior tests. Gates: build/vet/gofmt/test green.
- [ ] 1.4 Human output format + failure summary; document zero-config behavior. Gates: build/vet/gofmt/test green.

## 2. Compiled path (requires the M2 emitter — reference m2-emitter-v0 if open, do not edit it)

- [ ] 2.1 pkg/emit: route `deftest`/`defbench`/`^:test-only` top-level forms into sibling `<file>_test.go` per design D2, including the test-only TestMain registration shim; golden tests. Gates: build/vet/gofmt/test green.
- [ ] 2.2 cmd/cljgo: `test --compiled` drives emit + `go test` over the emitted tree, mapping results back to test vars; compile-and-run test proving a prod binary contains no test code. Gates: build/vet/gofmt/test green.

## 3. Dual harness for users

- [ ] 3.1 `--both`: run both modes, comparison keyed by qualified test var (outcome, assertion counts, normalized messages per design D4); DIVERGE output + summary + exit codes; tests covering agree/diverge/one-side-crash. Gates: build/vet/gofmt/test green.
- [ ] 3.2 Message-normalization rules implemented and themselves conformance-tested (path stripping, gensym masking). Gates: build/vet/gofmt/test green.
- [ ] 3.3 `--json` output for test results and `--both` diffs, consuming the structured-diagnostics schema if landed (documented fallback shape otherwise). Gates: build/vet/gofmt/test green.

## 4. Benchmarks + close-out

- [ ] 4.1 `defbench` registration + interpreted `--bench` harness; conformance file (cljgo extension, no JVM oracle — noted). Gates: build/vet/gofmt/test green.
- [ ] 4.2 Compiled `--bench`: emit `testing.B` Benchmark funcs, defer to `go test -bench`; wire one budget benchmark into CI per ADR 0004. Final gate: build/vet/gofmt/test green and the full conformance suite green in both modes.
