# Tasks — apply-adr-0033-hostfacts

## 1. The two call sites

- [x] 1.1 `pkg/emit/compile.go` `Build`: set `opts.HostFactsDir = genDir`
  unconditionally before `WriteModule`. Also `os.MkdirAll(genDir)` when a
  caller supplies a not-yet-existing `-gen` dir — `WriteModule`'s own
  `MkdirAll` runs after `EmitMain`'s fact load, so the dir must exist
  before that load or `go/packages.Load` fails with "no such file or
  directory" (caught by the regression test below). Gates green.
- [x] 1.2 `pkg/build/build.go` `buildArtifact`: move
  `opts.HostFactsDir = genDir` out of the `if len(p.GoRequires) > 0`
  block so it's always set before `WriteModule`. No dir-existence fix
  needed here — `genDir` always comes from `os.MkdirTemp`, which already
  creates it. Gates green.

## 2. Regression test

- [x] 2.1 `pkg/emit/hostfacts_binary_test.go`: `TestHostFactsNoNetworkForStdlib`
  proves stdlib fact resolution against a go.mod-less dir needs no
  network (`GOPROXY=off`); `TestBuildStdlibInteropOutsideRepo` builds a
  stdlib-only `(require-go '[strings])` program via `Build` from a temp
  dir outside the repo with `CLJGO_SRC` unset and cwd moved away from the
  repo (so `FindRuntimeDir()`'s walk-up can't accidentally succeed) —
  skips only the unrelated runtime-module proxy fetch on `isNetworkErr`.
  Gates green.

## 3. Verify existing coverage unaffected

- [x] 3.1 Conformance dual harness (in-repo path, calls `WriteModule`
  directly) still passes unchanged — full `go test ./...` green,
  including `conformance` (297s). Gates green.
