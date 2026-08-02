# apply-adr-0033-hostfacts

## Why

ADR 0033 (accepted, evidence: spike S17,
spikes/s17-hostfacts-from-binary/VERDICT.md) closes ADR 0028's flagged
non-goal: Go-interop host facts must resolve from the generated module,
not a source tree. S17 proved `go/packages` already resolves stdlib with
no go.mod at all, and third-party once the generated go.mod requires it
— the gap is exactly two call sites that never point `HostFactsDir` at
the generated module for the stdlib-only case. The spike already did the
design; this change applies it.

## What Changes

- `pkg/emit/compile.go` `Build`: sets `opts.HostFactsDir = genDir`
  unconditionally, before `WriteModule` runs `EmitMain`'s host-fact load.
  `genDir` need not contain a go.mod yet — stdlib resolves regardless
  (S17, measured, no latency delta).
- `pkg/build/build.go` `buildArtifact`: same — `opts.HostFactsDir = genDir`
  moves out of the `if len(p.GoRequires) > 0` gate so a stdlib-only
  `build.cljgo` artifact (no `go-require` calls) also resolves without
  falling through to `FindRuntimeDir()`.
- No change to `loadHostFacts` / `EmitMain`'s `packages.Config` — the
  mechanism already does the right thing once pointed at a directory.
  `FindRuntimeDir()` stays as `EmitMain`'s last-resort fallback, but after
  this change it is only reached by callers that don't set `HostFactsDir`
  at all (the `-runtime`/`CLJGO_SRC` override path and the in-repo
  conformance harness, which call `WriteModule` directly without going
  through `Build`/`buildArtifact`).
- Regression test (`pkg/emit`): build a `(require-go '[strings])`-only
  program from a temp dir outside the repo with `CLJGO_SRC` unset,
  mirroring `gomod_test.go`'s `TestBuildFromReleasePin` pattern — asserts
  stdlib interop needs no network (no proxy dependency, unlike the
  release-pin runtime fetch).

## Non-goals

- No change to third-party (`go-require`) ordering — `pkg/build/build.go`
  already synthesizes go.mod + `go get`s before `WriteModule` runs; this
  change only removes the gate on setting `HostFactsDir`, it doesn't
  reorder anything there.
- No change to `FindRuntimeDir()` itself or the `-runtime`/`CLJGO_SRC`
  override precedence (ADR 0028) — those keep working exactly as today.
- Multi-module workspace / `GOWORK` — out of scope per the spike.

## Capabilities

### Modified Capabilities
- `go-interop`: host fact resolution for the no-`go-require` interop path
  now always uses the generated module directory, never
  `FindRuntimeDir()`'s repo walk-up, when building through `cljgo build
  <file>` or a `build.cljgo` artifact.
