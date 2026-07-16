# ADR 0028 — `cljgo build` works from a downloaded binary: the runtime is a published Go module
Date: 2026-07-16 · Status: accepted · Evidence: spike S12 (spikes/s12-build-from-binary/VERDICT.md) · Supersedes the ADR 0013 v0 `replace` as the default

## Context

`cljgo build` required a repo checkout + `CLJGO_SRC` because the generated
go.mod `replace`d the runtime to a local tree. S12 proved the alternative:
a bare `require github.com/muthuishere/cljgo v0.1.0` builds and runs from
proxy.golang.org in a clean directory — no checkout, no env var. Measured:
one-time global proxy backfill 44.8s, then 0.6–3.7s cold; module zip 832KB;
build-time and binary-size deltas are noise. Emitted code imports only
pkg/emit/rt + pkg/lang (all-stdlib transitive closure, 2-line go.sum);
go:embed works from the module zip; the gitignored refs/ stub cannot poison.

## Decision

`SynthGoMod` (pkg/emit/program.go) picks the require/replace by binary kind:

1. **Release binary** (pkg/version.Version is a plain tag): write
   `require github.com/muthuishere/cljgo v<Version>` — no replace.
   Skew-safe by construction: the binary that emitted the code pins the
   runtime it was built from.
2. **Dev/dirty binary or explicit override**: today's replace path.
   Precedence: `-runtime` flag > `CLJGO_SRC` > release-pin > walk-up
   repo detection.
3. **Dev binaries must be distinguishable** (S12 gap): the in-source
   default becomes `0.1.0-dev`; release ldflags set the real tag (the
   hook exists — .goreleaser.yaml already stamps it).

## Consequences

A downloaded binary + the Go toolchain is the complete `cljgo build`
story; the README's "known v0 limitation" paragraph retires. Each release
must keep the emitted-code API surface (pkg/emit/rt + pkg/lang) compatible
with what its own tag serves — true today by construction.
