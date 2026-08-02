# build-distribution Specification

## Purpose
What a distributed cljgo binary can do on its own: how it determines whether it
is a release, what module version it pins into a generated `go.mod`, and the
single version it reports everywhere.
## Requirements
### Requirement: a binary is a release when it can prove which published tag it was built from

cljgo MUST treat a binary as a release exactly when it can name the published
tag it was built from, by either mechanism: a goreleaser-stamped
`version.Version` holding a plain `major.minor.patch`, or a real requested
module tag recorded by the Go toolchain in
`debug.ReadBuildInfo().Main.Version`. `version.ReleaseVersion()` MUST return
that tag as plain SemVer, or the empty string.

`(devel)`, Go-synthesized pseudo-versions, and any tag carrying a prerelease
qualifier MUST NOT be accepted. Nothing in this mechanism may invent a module
version that was never published: correctness rests on the toolchain's own
checksum-verified resolution of the requested tag.

A build from a source CHECKOUT MUST NOT be a release, whatever
`Main.Version` says. Go stamps `Main.Version` from the nearest VCS tag for a
plain local `go build` inside the repository, so the presence of any `vcs.*`
build setting MUST disqualify the binary — `go install module@vX.Y.Z` resolves
through the module proxy and records none.

#### Scenario: a `go install …@vX.Y.Z` binary builds a project with no source tree

- GIVEN cljgo installed with `go install github.com/muthuishere/cljgo/cmd/cljgo@vX.Y.Z`
- WHEN a freshly generated project is built with that binary, with no cljgo
  checkout anywhere and `CLJGO_SRC` unset
- THEN the build succeeds
- AND the generated `go.mod` requires `github.com/muthuishere/cljgo vX.Y.Z`
  with no `replace` directive

#### Scenario: a developer's own build still compiles against their working tree

- GIVEN a cljgo binary built with `go build` inside the repository, after a
  release tag exists in its history
- WHEN it generates a project's `go.mod`
- THEN the binary is NOT treated as a release
- AND the generated module points at the local runtime tree, so local changes
  to the runtime take effect

### Requirement: the emitted release pin names the resolved tag, never the in-source version

`SynthGoMod`'s release-pin branch MUST require `"v" + ReleaseVersion()`. It
MUST NOT require `"v" + Version`, which on a `go install`-obtained binary
would emit `v0.1.0-dev` — a tag that does not exist.

`pkg/version.Version` MUST remain `"0.1.0-dev"` in source; no release
introduces a constant to bump.

#### Scenario: the pin is resolvable

- GIVEN a release binary at tag vX.Y.Z
- WHEN it synthesizes a project's go.mod
- THEN the required version is `vX.Y.Z`, which the module proxy can resolve

### Requirement: one version reporter, so the reported numbers cannot disagree

Every version reporter — `cljgo version`, `(cljgo-version)`,
`*cljgo-version*`, and the nREPL handshake — MUST read `version.Self()`, which
returns the published tag when there is one and the in-source `Version`
otherwise.

#### Scenario: CLI and runtime agree on a `go install`ed binary

- GIVEN cljgo installed at tag vX.Y.Z via `go install`
- WHEN `cljgo version` and `(cljgo-version)` are both consulted
- THEN both report `X.Y.Z`
- AND neither describes the binary as a dev build

### Requirement: runtime resolution has one precedence order, and an override always wins

The synthesized `go.mod` MUST resolve the runtime tree by exactly this
precedence: the `-runtime` flag, then `$CLJGO_SRC`, then the release pin, then
walk-up repository detection.

An explicit `-runtime` directory or a set `$CLJGO_SRC` MUST force a local
`replace` **even in a release binary**. A developer who has said where the
runtime is MUST NOT be silently compiled against the published module
instead — that failure has no symptom, because the build succeeds and only
the behaviour is wrong.

A dev binary MUST locate the runtime tree by walk-up, and MUST fail with an
actionable error when none is found, naming both remedies.

#### Scenario: an explicit override beats the release pin

- GIVEN a release binary
- WHEN it generates a module with `-runtime <dir>` given, or `$CLJGO_SRC` set
- THEN the generated `go.mod` carries a `replace` to that tree, not the pin

#### Scenario: a dev binary inside the repository builds offline

- GIVEN a source-built binary generating a module inside the repository
- WHEN no override is set
- THEN `go.mod` carries `replace github.com/muthuishere/cljgo => <repo>`
- AND the build requires no network

#### Scenario: a dev binary with no runtime tree fails actionably

- GIVEN a dev binary outside any checkout with `$CLJGO_SRC` unset
- WHEN it generates a module
- THEN the error states it is a dev build, and how to fix it — set
  `$CLJGO_SRC` or run inside the repository

### Requirement: a pinned module gets a go.sum before it is built

`GoBuild` MUST run `go mod tidy` in a generated module that requires the
runtime by version with no `replace` and has no `go.sum` yet.

A replace-based module MUST NOT invoke tidy: it has no network dependency,
and adding one would put a network round-trip inside the conformance
harness's perf budget. `go.mod` stays user-owned once written.

#### Scenario: a replace-based module builds offline

- GIVEN a replace-based generated module
- WHEN it is built
- THEN no `go mod tidy` runs
- AND the build needs no network

