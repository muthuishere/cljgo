# build-from-binary

## Why

ADR 0028 (accepted, evidence: spike S12) retires the v0 requirement that
`cljgo build` needs a repo checkout + `CLJGO_SRC`. S12 proved a bare
`require github.com/muthuishere/cljgo v0.1.0` builds and runs from
proxy.golang.org in a clean directory — the only change needed is what
`SynthGoMod` writes into go.mod, plus a dev marker so a source-built binary
is distinguishable from a release binary. This change applies the ADR; the
spike already did the design (spikes/s12-build-from-binary/VERDICT.md).

## What Changes

- `pkg/version`: in-source default becomes `0.1.0-dev`; new `IsRelease()`
  reports whether Version is a plain `major.minor.patch` tag (release
  ldflags — .goreleaser.yaml — stamp the real tag, unchanged).
- `SynthGoMod` (pkg/emit/program.go) picks the go.mod shape by precedence
  `-runtime` flag > `CLJGO_SRC` > release-pin > walk-up repo detection:
  release binaries write `require github.com/muthuishere/cljgo v<Version>`
  with no replace; everything else keeps today's local `replace`.
- `GoBuild` runs `go mod tidy` before building a release-pinned module
  (a bare require needs go.sum entries); replace-based dev modules are
  untouched — no network, no perf-budget impact on the conformance harness.
- README: the "known v0 limitation" paragraph retires — honestly phrased as
  "from v0.2.0", since shipped v0.1.0 binaries keep the old behavior.

## Non-goals

- No version bumps; no release in this change.
- Go-interop host-fact loading (`loadHostFacts`) still resolves against a
  local runtime tree when no generated-module dir is available — making
  interop programs build from a downloaded binary alone is follow-up work,
  not part of ADR 0028's SynthGoMod scope.
- No runtime-only module split (S12 zip-hygiene note — explicitly deferred).

## Capabilities

### New Capabilities
- `build-distribution`: how the generated module resolves the cljgo runtime —
  release-pin vs local-replace selection, the dev version marker, and the
  go.sum step.

### Modified Capabilities
(none — openspec/specs/ has no capability covering go.mod synthesis)

## Impact

- pkg/version (default + IsRelease + tests), pkg/emit/program.go
  (SynthGoMod, GoBuild), cmd/cljgo (-runtime flag help text), README.md.
- Conformance dual harness: unchanged — in-repo dev binaries keep the
  walk-up replace path.
