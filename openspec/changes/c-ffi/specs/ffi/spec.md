## ADDED Requirements

### Requirement: purego is the one FFI mechanism, two registration strategies
`ffi/deflib` SHALL bind C functions via purego (`Dlopen`/`Dlsym`/
`RegisterLibFunc`/`RegisterFunc`) using a STATIC (compile-time-typed func
var) strategy when the declaration is comptime-visible to the analyzer,
and a DYNAMIC (`reflect.FuncOf` + `reflect.New` + `RegisterFunc`) strategy
for the interpreter and for genuinely runtime-constructed declarations
(`ffi/fn`). Both strategies SHALL produce the same observable Clojure fn
value semantics (ADR 0044 #1).

#### Scenario: dynamic registration with no compile-time Go signature
- **WHEN** `(ffi/deflib libm "libm.dylib" (cos "cos" [:double] :double))`
  is evaluated at the REPL
- **THEN** `libm/cos` is callable and returns the C library's result,
  with no Go source anywhere declaring a `func(float64) float64`

#### Scenario: dual-mode parity
- **WHEN** the identical `ffi/deflib` form runs interpreted and as an
  AOT-compiled binary
- **THEN** both report the identical result for the same call (ADR 0007)

### Requirement: REPL-liveness — no rebuild between declarations
Re-evaluating an `ffi/deflib` (or `ffi/fn`) form in the same running
interpreter process SHALL take effect immediately for subsequent calls,
with no process restart.

#### Scenario: re-declare changes behavior live
- **WHEN** a var bound by `ffi/deflib` is re-declared to a different C
  symbol in the same session
- **THEN** a call issued after the re-declaration observes the new
  symbol's behavior, and a call issued before it observed the old one

### Requirement: declaration-time failures are named and total
A missing library, a missing symbol, or a variadic C declaration SHALL
fail at `ffi/deflib` declaration time with a positioned error naming the
library/fn/symbol involved, and SHALL leave no function from that
declaration bound (all-or-nothing per lib).

#### Scenario: missing library
- **WHEN** `ffi/deflib` names a library path that does not resolve
- **THEN** declaration fails with an error naming the library and path,
  and no fn in that `deflib` form is bound

#### Scenario: missing symbol
- **WHEN** one declared fn's C symbol does not exist in an otherwise
  valid library
- **THEN** declaration fails with an error naming both the C symbol and
  the Clojure fn name, and no fn in that `deflib` form is bound

#### Scenario: variadic declaration rejected
- **WHEN** a decl is marked variadic
- **THEN** declaration fails with an error citing ADR 0011/S7's finding
  (purego's `...any` is not C varargs) and suggesting a cgo/Go wrapper

### Requirement: call-time arity/type errors, never a raw panic
Calling a bound ffi fn with the wrong number of arguments, or an argument
not assignable to its declared keyword's Go type, SHALL fail with a named
error identifying the fn and the mismatch, checked before the underlying
purego call executes.

#### Scenario: wrong arity
- **WHEN** a fn declared with N args is called with a different count
- **THEN** the call returns an error naming the fn and both counts,
  and no C call occurs

### Requirement: wrong-signature corruption is a documented limitation, not a guarantee
A declared type that does not match the C library's real signature (right
symbol, wrong types) SHALL NOT be detected by `ffi/deflib` — it is an ABI
register-class mismatch outside any marshaling layer's visibility. This
SHALL be documented, not silently promised away.

#### Scenario: wrong signature returns a wrong value, not an error
- **WHEN** a fn is declared with a return/arg keyword that does not match
  the C symbol's real type (e.g. `:int32` where the C fn is `double`)
- **THEN** the call returns a value with no error, and that value SHALL
  NOT be assumed correct

### Requirement: dependency placement rides the per-program module
The `cljgo` interpreter/REPL binary SHALL depend on `purego` unconditionally.
A compiled program's own generated `go.mod` SHALL depend on `purego` if and
only if that program's source uses `ffi/`.

#### Scenario: FFI-free program stays zero-third-party-deps
- **WHEN** a program with no `ffi/` usage is built with `cljgo build`
- **THEN** its emitted `go.mod` has no `purego` requirement

#### Scenario: FFI-using program gains exactly the one dependency
- **WHEN** a program uses `ffi/deflib` or `ffi/fn`
- **THEN** its emitted `go.mod` requires `purego` and nothing else new

### Requirement: platform claim matches purego's Tier 1
cljgo SHALL document `ffi/deflib` as fully supported on darwin, linux, and
windows, each on amd64 and arm64, and as best-effort/untested elsewhere,
matching purego's own support tiers.

#### Scenario: unsupported-tier behavior is documented, not silently degraded
- **WHEN** `ffi/deflib` is used on a platform outside the documented Tier 1
  set
- **THEN** the reference doc states the support level as best-effort, not
  conformance-tested
