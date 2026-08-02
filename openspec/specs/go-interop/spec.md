# go-interop Specification

## Purpose
TBD - created by archiving change m3-interop-v0. Update Purpose after archive.
## Requirements
### Requirement: Go packages are required as namespaces
The system SHALL let `require-go` (REPL) and `:require-go` (in `ns`) bind a Go
package to a Clojure namespace alias, after which `alias/Member` references the
package's exported members, identically in interpreted and compiled modes. The
path may be a single-segment symbol or a string; the default alias is the last
`/`-segment; `:as` overrides it.

#### Scenario: package fn call
- **WHEN** `(require-go '[strings])` then `(strings/ToUpper "hi")` is evaluated
- **THEN** the result is `"HI"`

#### Scenario: const access in value position
- **WHEN** `(require-go '[math])` then `math/Pi` is evaluated
- **THEN** the result is `3.141592653589793`

#### Scenario: alias override
- **WHEN** `(require-go '[strconv :as sc])` then `(sc/Itoa 42)` is evaluated
- **THEN** the result is `"42"`

### Requirement: Clojure resolution takes precedence over host aliases
The system SHALL resolve a namespaced symbol as a Clojure var whenever its
namespace names a Clojure namespace or alias, and only fall back to Go host
resolution otherwise — a `:require-go` alias never shadows Clojure.

#### Scenario: a Clojure namespace is never overridden
- **WHEN** a symbol resolves to both a Clojure namespace and a would-be host alias of the same name
- **THEN** the Clojure var is used

### Requirement: (T, error) returns shape to [v err], with ! to throw
The system SHALL map a Go function's trailing `error` result to a 2+-element
Clojure vector `[v err]` on a plain call (error slot nil-normalized), and, when
the operator carries a `!` suffix, SHALL return the value alone and throw when
the error is non-nil. Comma-ok `(T, bool)` maps to `[v ok]` the same way. This
shaping is byte-identical across interpreted and compiled modes.

#### Scenario: plain call yields a value/error vector
- **WHEN** `(require-go '[strconv])` then `(strconv/Atoi "42")` is evaluated
- **THEN** the result is the vector `[42 nil]`

#### Scenario: error is a truthy value in the error slot
- **WHEN** `(strconv/Atoi "x")` is evaluated
- **THEN** the result is a 2-element vector `[0 <err>]` whose first element is the Go zero value passed through (`0`, exactly what `v, err := strconv.Atoi("x")` binds so AOT and eval agree) and whose second element is truthy (a non-nil error)

#### Scenario: bang unwraps on success
- **WHEN** `(strconv/Atoi! "42")` is evaluated
- **THEN** the result is `42`

#### Scenario: bang throws on failure
- **WHEN** `(strconv/Atoi! "x")` is evaluated
- **THEN** an exception is thrown

### Requirement: host return values are normalized to Clojure numbers and nil
The system SHALL widen Go integer results to `int64` and floating results to
`float64`, and SHALL present a typed-nil pointer/interface/map/slice/chan/func
result as Clojure `nil`, in both modes.

#### Scenario: integer result prints as a Clojure integer
- **WHEN** `(require-go '[strconv])` then `(strconv/Atoi! "7")` is evaluated
- **THEN** the result prints as `7`

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

