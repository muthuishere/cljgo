## 1. Representation spike (freezes D1 before anything ships)

- [ ] 1.1 Spike S1: benchmark tagged-struct vs 2-elem-vector vs keyword-tagged-map representations (construct/predicate/unwrap/let?-chain-of-5) with Go benchmarks; record numbers + allocation counts against the ADR 0004 budget in the change; pick the winner. Gates: `go build ./... && go vet ./... && gofmt -l pkg cmd conformance && go test ./...` green.

## 2. Core primitives (pkg/lang + core/)

- [ ] 2.1 pkg/lang: Result/Option types per the spiked representation, singletons, `=`/hash entries in the value tables (design/00 §4.2); unit tests including nil-payload cases. Gates: build/vet/gofmt/test green.
- [ ] 2.2 core/: constructors `ok`/`err`/`just`/`none`, predicates, `unwrap` (ex-info bridge), `unwrap-or`, `map-ok`, `map-err`, `and-then`; conformance/tests/*.clj with frozen expectations (exception-bridge behavior of ex-info/ex-data verified against real JVM Clojure 1.12.5 and cited; the primitives themselves documented as a cljgo extension). Gates: build/vet/gofmt/test green.
- [ ] 2.3 Printer/reader: `#cljgo/ok|err|just|none` tagged literals in pkg/lang printing and the reader (both modes); round-trip conformance file (printing of payload data verified against the real `clojure` CLI printer). Gates: build/vet/gofmt/test green.

## 3. let? and lint

- [ ] 3.1 core/: `let?` macro per design D5 including destructuring-after-unwrap; conformance file covering short-circuit, success path, plain-binding passthrough, nil-vs-none. Gates: build/vet/gofmt/test green.
- [ ] 3.2 pkg/analyzer: opt-in discarded-Result warning as a W-band structured diagnostic (coordinates with the structured-diagnostics change for schema; if not yet landed, emit through the existing warning path and note the follow-up). Gates: build/vet/gofmt/test green.

## 4. Interop lift (M3 interop must exist)

- [ ] 4.1 Interpreted interop: `:result` call variant beside `!` (ADR 0005 path untouched); conformance file driving a real Go stdlib call through ok and err branches. Gates: build/vet/gofmt/test green.
- [ ] 4.2 AOT interop: pkg/emit emits the lift with no reflection and no extra allocation beyond the tagged value itself; golden test + compile-and-run test. Gates: build/vet/gofmt/test green.
- [ ] 4.3 Dual-harness: all result-option conformance files through both paths; divergence is a release blocker (design/03 §7d). Update design/05 §2 docs cross-reference and flip ADR 0014 status to accepted (status-line edit only). Final gate: build/vet/gofmt/test green, benchmarks within budget.
