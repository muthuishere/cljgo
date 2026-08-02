> **Rewritten 2026-08-02 against ADR 0116.** The original delta defined a
> release as "`IsRelease()` — Version is a plain `major.minor.patch` tag" and
> required the pin to be `v<Version>`. ADR 0116 superseded both: `IsRelease()`
> is now `ReleaseVersion() != ""` (`pkg/version/version.go:148`), and a
> `go install …@vX.Y.Z` binary is a release whose `Version` is still
> `0.1.0-dev`. Archiving the original text would have merged a requirement
> contradicting this capability's own live spec, which forbids `"v" + Version`
> by name. Release detection and pinning are stated there and are NOT restated
> here; what remains below is only what ADR 0028 decided and nothing has
> superseded.

## ADDED Requirements

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
