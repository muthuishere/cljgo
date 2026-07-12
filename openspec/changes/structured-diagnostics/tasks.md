## 1. pkg/diag foundation

- [ ] 1.1 pkg/diag: Diagnostic struct per design D1, banded code type, registry.go with typed entries, and the two renderers' interfaces; unit tests. Gates: `go build ./... && go vet ./... && gofmt -l pkg cmd conformance && go test ./...` green.
- [ ] 1.2 Append-only machinery: go:generate for docs/diagnostics/registry.md, registry.lock snapshot, tests failing on removal/renumber/summary-change and on missing explain pages. Gates: build/vet/gofmt/test green.
- [ ] 1.3 Golden tests freezing today's human error text for existing reader/analyzer errors (the owner's unchanged-by-default constraint) BEFORE any rewiring. Gates: build/vet/gofmt/test green.

## 2. Rewire producers + --json

- [ ] 2.1 pkg/reader: CompilerError construction through pkg/diag with R-band codes + explain pages for each; golden human-text tests still pass. Gates: build/vet/gofmt/test green.
- [ ] 2.2 pkg/analyzer (and pkg/eval compile-error paths): A-band codes, spans from AST positions (design/03 §1), related[] notes where the analyzer knows them (e.g. recur target); explain pages. Gates: build/vet/gofmt/test green.
- [ ] 2.3 cmd/cljgo: global `--json` flag rendering the cljgo-diag/1 envelope on stderr for every verb, exit codes unchanged; JSON-shape tests including UTF-8 byte_range correctness. Gates: build/vet/gofmt/test green.
- [ ] 2.4 First fixes[]: attach machine-applicable fixes to at least three high-frequency diagnostics (with byte-range application test) — the M2 forcing-function starts here. Gates: build/vet/gofmt/test green.

## 3. Debug API — endpoints

- [ ] 3.1 Endpoint table + core implementations: check, get_symbols (design/03 §3b namespaces/vars), get_ast (AST → data with positions); unit tests. Gates: build/vet/gofmt/test green.
- [ ] 3.2 explain (serves docs/diagnostics pages) and suggest_fix (per-run diagnostic id → fixes[]); source-content-hash echo per design risk note. Gates: build/vet/gofmt/test green.
- [ ] 3.3 Stubs for get_types/get_control_flow/get_data_flow returning the structured D-band not-yet-available answer; tests assert schema conformance of stubs. Gates: build/vet/gofmt/test green.

## 4. Debug API — transports + close-out

- [ ] 4.1 `cljgo debug` REPL: clojure.compiler namespace generated from the endpoint table, returning Clojure data; verify absent from default REPL and emitted binaries (symbol-absence test). Gates: build/vet/gofmt/test green.
- [ ] 4.2 `cljgo debug --stdio`: NDJSON request/response loop from the same endpoint table; transport-agreement test (same source → same diagnostics both transports). Gates: build/vet/gofmt/test green.
- [ ] 4.3 E-band reservation handed to the M2 emitter change (m2-emitter-v0 if open — reference, do not edit) and I-band to the interop path; docs for the agent check→explain→fix loop; flip ADR 0015 status to accepted (status-line edit only). Final gate: build/vet/gofmt/test green and full conformance suite green in both modes.
