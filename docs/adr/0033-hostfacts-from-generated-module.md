# ADR 0033 — Interop host facts resolve from the generated module, not a source tree
Date: 2026-07-16 · Status: accepted · Evidence: spike S17 (spikes/s17-hostfacts-from-binary/VERDICT.md) · Closes ADR 0028's flagged non-goal

## Context

Interop `cljgo build` from a downloaded binary failed: fact loading fell
through to FindRuntimeDir(). S17 proved the mechanism already works without
any cljgo checkout — stdlib resolves via GOROOT with no go.mod at all
(negative control confirmed), third-party resolves once the generated
go.mod requires it, and there is no latency penalty (48ms vs 80ms baseline
for stdlib). The gap is exactly two call sites that never set HostFactsDir
for the stdlib-only case.

## Decision

- `pkg/emit/compile.go` Build and `pkg/build/build.go` buildArtifact set
  `opts.HostFactsDir = genDir` unconditionally (not gated on GoRequires).
- loadHostFacts / EmitMain's packages.Config: unchanged.
- FindRuntimeDir() remains only for explicit `-runtime`/`CLJGO_SRC`
  overrides and the in-repo dev/conformance harness.
- Regression test: a `(require-go '[strings])` program built from a temp
  dir outside the repo with CLJGO_SRC unset.

## Consequences

With ADR 0028, a downloaded release binary + the Go toolchain is the
COMPLETE cljgo story, interop included. The adoption path has no
checkout anywhere.
