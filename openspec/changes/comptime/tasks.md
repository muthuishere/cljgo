## 1. Analyzer + shared plumbing

- [ ] 1.1 Add `comptime`, `comptime-assert`, `embed-file` as special forms in pkg/analyzer (new AST ops, position capture, arity checks); unit tests. Gates: `go build ./... && go vet ./... && gofmt -l pkg cmd conformance && go test ./...` green.
- [ ] 1.2 Implement the embeddability checker (design D2 table, cycle-safe walk) as a shared pkg (usable by eval and emit) with table-driven unit tests covering every row. Gates: build/vet/gofmt/test green.

## 2. Interpreted mode

- [ ] 2.1 pkg/eval: inline evaluation for all three forms per design D4 (embeddability check on result, comptime-assert throws with position, embed-file reads relative to source). Gates: build/vet/gofmt/test green.
- [ ] 2.2 Conformance: `conformance/tests/comptime_*.clj` files with frozen `;; expect:` outputs; comments note comptime is a cljgo extension (no JVM oracle) and cite the JVM-checked printer behavior used for embeddable-value round-trips (verify printing against real `clojure` CLI). Gates: build/vet/gofmt/test green.

## 3. AOT mode (layers on the M2 emitter — reference m2-emitter-v0 if open, do not edit it)

- [ ] 3.1 pkg/emit: compile-time evaluation hook for comptime bodies via the linked evaluator; failures surface as positioned diagnostics. Gates: build/vet/gofmt/test green.
- [ ] 3.2 pkg/emit: literal emitter for every embeddable class (deterministic ordering for maps/sets, interning per design/00 §4.4); golden tests. Gates: build/vet/gofmt/test green.
- [ ] 3.3 pkg/emit: `comptime-assert` build failure path and `embed-file` (string + :bytes) emission; byte-array representation micro-spike (string-const vs []byte literal) with benchmark recorded against the ADR 0004 budget. Gates: build/vet/gofmt/test green.
- [ ] 3.4 Dual-harness: run the comptime conformance files through both paths; any REPL-vs-binary divergence is a release blocker (design/03 §7d). Gates: build/vet/gofmt/test green.

## 4. Build-cache honesty + docs gate

- [ ] 4.1 I/O recorder in the compile-time evaluator (file opens + env reads → hashed cache-key inputs) and cmd/cljgo cache-key wiring; `--no-comptime-cache` flag; test: editing an embedded file invalidates the cache. Gates: build/vet/gofmt/test green.
- [ ] 4.2 Docs: macros-vs-comptime guidance (ADR 0009 §4) and the embeddability table published; flip ADR 0009 status line to accepted (one-line status edit, not history rewrite). Final gate run: build/vet/gofmt/test green and full conformance suite green in both modes.
