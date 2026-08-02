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

