## ADDED Requirements

### Requirement: Release binaries pin the published runtime module
When the emitting binary is a release build (`pkg/version.IsRelease()` —
Version is a plain `major.minor.patch` tag) and no runtime override is in
effect, the synthesized go.mod SHALL contain
`require github.com/muthuishere/cljgo v<Version>` and SHALL NOT contain a
`replace` directive. The pin SHALL be the emitting binary's own version, so
emitted-code API skew is impossible by construction.

#### Scenario: downloaded binary, clean directory
- **WHEN** a release binary (Version `0.1.0`) runs `cljgo build hello.clj`
  outside any cljgo checkout with `CLJGO_SRC` unset
- **THEN** the generated go.mod requires `github.com/muthuishere/cljgo v0.1.0`
  with no replace, and `go build` succeeds fetching the runtime from the Go
  module proxy

#### Scenario: unpublished tag fails loudly
- **WHEN** the pinned version does not exist on the proxy
- **THEN** the build fails with Go's own `unknown revision` diagnostic — no
  silent wrong-version build

### Requirement: Overrides and dev binaries keep the local replace
The synthesized go.mod SHALL use today's local `replace` path with precedence
`-runtime` flag > `CLJGO_SRC` > release-pin > walk-up repo detection: an
explicit `-runtime` dir or a set `CLJGO_SRC` SHALL force a replace even in a
release binary, and a dev binary (Version carries a qualifier, default
`0.1.0-dev`) SHALL locate the runtime tree by walk-up as before, erroring
helpfully when none is found.

#### Scenario: dev binary in the repo (conformance harness)
- **WHEN** a source-built binary (Version `0.1.0-dev`) generates a module
  inside the repo
- **THEN** go.mod carries `replace github.com/muthuishere/cljgo => <repo>`
  exactly as today, offline

#### Scenario: explicit override beats the release pin
- **WHEN** a release binary is given `-runtime <dir>` or `CLJGO_SRC`
- **THEN** go.mod replaces the runtime to that tree

#### Scenario: dev binary with no runtime tree
- **WHEN** a dev binary generates a module outside any checkout with
  `CLJGO_SRC` unset
- **THEN** the error says it is a dev build and how to fix it (set
  `CLJGO_SRC` or run inside the repo)

### Requirement: Release-pinned modules get go.sum before go build
`GoBuild` SHALL run `go mod tidy` in a generated module whose go.mod requires
the runtime by version without a replace and which has no go.sum yet;
replace-based modules SHALL NOT invoke tidy (no network dependency, no
conformance perf-budget impact). go.mod stays user-owned once written.

#### Scenario: replace-based module builds offline
- **WHEN** a replace-based generated module is built
- **THEN** no `go mod tidy` runs and the build needs no network
