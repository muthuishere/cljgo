## 1. Contract handoff (before/with the M2 emitter)

- [ ] 1.1 Write the munging public-contract page (design D2 table: char munges, ns→path, export casing, provenance comments, collision rule) and verify identifier legality against Go's go/token rules with a table-driven test; hand the requirements to the M2 emitter change (m2-emitter-v0 if open — reference, do not edit). Gates: `go build ./... && go vet ./... && gofmt -l pkg cmd conformance && go test ./...` green.
- [ ] 1.2 pkg/emit: export-surface resolution (`^:export` union project-file exports map; missing-var error; casing-collision error naming both vars); unit tests. Gates: build/vet/gofmt/test green.

## 2. Go library output

- [ ] 2.1 pkg/emit: exported-fn emission — hinted real signatures with boxed-`any` fallback, docstring→doc comment, provenance comments, unexported internals; golden tests. Gates: build/vet/gofmt/test green.
- [ ] 2.2 cmd/cljgo: `build --lib` — target/lib module dir, `:go-module` requirement + error, go.mod with pinned runtime version, generated-code headers, determinism test (byte-identical double build). Gates: build/vet/gofmt/test green.
- [ ] 2.3 End-to-end test: emit a lib, consume it from a plain Go program with `go build` only, compare returned values against the interpreted result (dual-harness spirit, design/03 §7d). Gates: build/vet/gofmt/test green.

## 3. C library output

- [ ] 3.1 pkg/emit: C-expressibility classifier for hinted signatures + `//export` cgo wrapper generation (string copy semantics, ptr+len for bytes) + exclusion warnings; unit tests. Gates: build/vet/gofmt/test green.
- [ ] 3.2 cmd/cljgo: `--c-shared`/`--c-archive` buildmode driving with CGO_ENABLED=1 passthrough (design/00 §1.5); ship cljgo_runtime.h (CljgoInit idempotence test). Gates: build/vet/gofmt/test green.
- [ ] 3.3 End-to-end test: C program links the .so via generated header, calls through CljgoInit, value verified; runs in CI on at least linux. Gates: build/vet/gofmt/test green.

## 4. Clojure-library mode + close-out

- [ ] 4.1 Document + test source consumption by another cljgo project via path/git deps (interpreted and AOT'd by the consumer); conformance file for cross-project require. Gates: build/vet/gofmt/test green.
- [ ] 4.2 User docs: the four build modes from one codebase, publishing recipe (commit/tag emitted source), stability guarantees; verify all doc examples compile. Final gate: build/vet/gofmt/test green and full conformance suite green in both modes.
