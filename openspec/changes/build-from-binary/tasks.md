## 1. Version surface

- [x] 1.1 pkg/version: default `Version` becomes `0.1.0-dev`; add `IsRelease()` (true iff plain `major.minor.patch`, no qualifier); `version_test.go` covers both shapes including the ldflags-stamped release shape. Gates: `go build ./... && go vet ./... && gofmt -l pkg cmd conformance && go test ./...` green.

## 2. go.mod synthesis

- [x] 2.1 pkg/emit `SynthGoMod`: require-vs-replace by precedence `-runtime` flag > `CLJGO_SRC` > release-pin (`require github.com/muthuishere/cljgo v<Version>`, no replace) > walk-up; helpful dev-binary error when nothing resolves; `GoBuild` runs `go mod tidy` only for release-pinned modules with no go.sum. Unit tests for every branch (offline). Gates green.
- [x] 2.2 End-to-end require-path test (the S12 replay): with Version set to `0.1.0`, generate a module in a temp dir with no CLJGO_SRC, assert the bare require, `go build` against the proxy, run the binary. Skips under `-short` and on network failure (pkg/build isNetworkErr pattern). Gates green.

## 3. Docs

- [x] 3.1 README: retire the "known v0 limitation" paragraph — phrased as "from v0.2.0" (v0.1.0 binaries keep the old behavior); `-runtime` flag help text updated. No version bumps. Final gates + full conformance suite green in both modes.
