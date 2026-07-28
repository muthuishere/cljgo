# Tasks — adr-0106-cljx-core

Gate: `CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates core && go test ./... -timeout 1800s -p 1`

- [ ] 1.1 `core/cljx/core.cljg` — satellite preamble; add!/del!/bump!/upd!/put-in!/upd-in!/clear!/toggle!/dbg + dbg->/dbg->>. Docstrings MUST show the swap! form each replaces.
- [ ] 1.2 Embed in `core/cljx.go`; Specs() row LAST (`cljx.core`, Pkg `cljxcore`, install nil); `go generate ./pkg/briaot`.
- [ ] 1.3 Conformance `conformance/tests/cljx-core-*.clj`: each helper proven EQUIVALENT to its swap! form (assert both produce the same value), bump!-from-absent, dbg transparency in a pipeline, clear! preserving collection type. Dual harness. oracle: n/a — cljgo namespace; freeze cljgo behavior with rationale.
- [ ] 1.4 Grep-gate: no name in cljx.core shadows clojure.core (assert programmatically in a test, not by eye).
- [ ] 2.1 Docs: book chapter "Less typing" teaching each verb beside the idiom it replaces + the honest JVM-portability note.
- [ ] 2.2 Close-out: full gate green; archive this change.
