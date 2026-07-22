# host-resolution-parity Specification

## Purpose
TBD - created by archiving change apply-adr-0049-host-parity. Update Purpose after archive.
## Requirements
### Requirement: Host references resolve identically or hard-error, never silently diverge

A host reference that the interpreter and the AOT-compiled binary would resolve
differently MUST hard-error in the leg that cannot satisfy it. The system MUST
NOT let one leg silently resolve such a reference to `nil`, `""`, `false`, or a
no-op while the other produces a real value.

#### Scenario: Third-party go-require member is unlinked in the interpreter
- **WHEN** a program `require-go`s a third-party (domain-dotted) Go module and
  accesses a member of it under `cljgo run`, and that module is not present in
  the reflect-seed registry
- **THEN** the interpreter raises an error naming the module path and the member
  (e.g. `go module <path> is not linked into the interpreter (accessing member
  <M>) (at <file>); build it (cljgo build), or use the self-rebuild flow`),
  and MUST NOT return `nil`

#### Scenario: Stdlib and cljgo-own members are unaffected
- **WHEN** a program accesses a member of a stdlib or cljgo-own Go package that
  is present in the registry
- **THEN** it resolves to the real value with no error

#### Scenario: A genuinely-nil Clojure value is not mistaken for an unlinked member
- **WHEN** an expression legitimately evaluates to `nil` (e.g. `(get {} :x)`, a
  var bound to `nil`)
- **THEN** it returns `nil` with no error — the unlinked-member detection MUST
  key off a registry miss, not the `nil` value

### Requirement: The unlinked-member error is suppressed during AOT namespace discovery

Because `cljgo build` discovers namespaces by evaluating require forms through
the interpreter, the unlinked-member hard-error MUST be suppressible so that
compiling a program that uses third-party Go modules is not itself broken.

#### Scenario: Emitter tolerates unlinked members during discovery
- **WHEN** the emitter runs the namespace-discovery pass with
  `Evaluator.HostUnlinkedTolerant` set to `true`
- **THEN** an unlinked third-party member access does not raise, allowing the
  build to proceed; the compiled binary links the module and resolves it

#### Scenario: run and REPL do not tolerate
- **WHEN** the same forms run under `cljgo run` or the REPL (`HostUnlinkedTolerant`
  default `false`)
- **THEN** the unlinked-member access hard-errors

### Requirement: Entry-namespace `*file*` and `require` behave consistently in a binary

An AOT-compiled binary MUST bind the entry namespace's `*file*` to its logical
source path, and MUST hard-error on `require` of a namespace not compiled into
the binary, rather than diverging from the interpreter.

#### Scenario: Entry `*file*` is the logical source path, not NO_SOURCE_FILE
- **WHEN** the entry namespace reads `*file*` inside an AOT binary
- **THEN** it is the namespace's logical source path (matching the interpreter's
  semantics), not `NO_SOURCE_FILE`

#### Scenario: Requiring an uncompiled namespace in a binary hard-errors
- **WHEN** a binary evaluates `(require 'some.ns)` for a namespace not compiled
  into it
- **THEN** it hard-errors naming the namespace, rather than silently no-op'ing

### Requirement: The dual-harness parity gate accepts three outcomes and forbids silent divergence

The conformance harness MUST run each parity case under both legs and accept
exactly one of three outcomes, failing only on a silent divergence.

#### Scenario: Accepted outcomes
- **WHEN** a parity case runs interpreted and AOT-compiled
- **THEN** the gate passes if the two legs produce identical output, OR identical
  error, OR the interpreter hard-errors naming an unavailable capability while
  the AOT leg succeeds

#### Scenario: Forbidden outcome
- **WHEN** the two legs produce different non-error values, or one leg silently
  yields `nil`/`""`/`false`/a no-op while the other produces a real value
- **THEN** the gate fails

