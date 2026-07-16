## MODIFIED Requirements

### Requirement: Host fact resolution needs no source checkout
The system SHALL resolve Go-interop host type facts (design/05 §2) against
the generated module directory for both the single-file `cljgo build
<file>` path and the `build.cljgo` artifact path, regardless of whether
the program uses `go-require`. `FindRuntimeDir()`'s repo walk-up SHALL be
reached only when a caller does not point host-fact resolution at a
generated module at all (an explicit `-runtime`/`CLJGO_SRC` override, or
the in-repo conformance harness calling `WriteModule` directly).

#### Scenario: stdlib-only interop from a downloaded binary
- **WHEN** a release binary with no `CLJGO_SRC` and no local cljgo
  checkout runs `cljgo build hello.clj` on a program containing only
  `(require-go '[strings])` (no `go-require` in play)
- **THEN** the build succeeds — host facts resolve against the generated
  module directory, not a source tree, and no network call is made

#### Scenario: stdlib-only artifact via build.cljgo
- **WHEN** a `build.cljgo` artifact has no `go-require` calls but its main
  file uses Go stdlib interop
- **THEN** `buildArtifact` still resolves the stdlib host facts against
  the generated module directory, not `FindRuntimeDir()`

#### Scenario: third-party interop via go-require is unaffected
- **WHEN** a `build.cljgo` artifact declares `go-require` for a
  third-party module
- **THEN** the existing ordering (synthesize go.mod with the pin, `go
  get`, then resolve facts against the generated module) is unchanged
