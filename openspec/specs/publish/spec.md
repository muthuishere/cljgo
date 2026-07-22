# publish Specification

## Purpose
TBD - created by archiving change apply-adr-0054-publish. Update Purpose after archive.
## Requirements
### Requirement: `cljgo publish` produces a library for a named ecosystem from one build.cljgo

`cljgo publish <target>` MUST produce a publishable artifact for `go` or
`clojars` from the single `build.cljgo` (ADR 0021), introducing no second
manifest. `publish go` MUST produce a go-gettable Go package; `publish clojars`
MUST produce pure Clojure source consumable via a `deps.edn` git coordinate.

#### Scenario: publish go emits a go-gettable package
- **WHEN** a library is published with `cljgo publish go`
- **THEN** a Go package is produced with Go signatures derived from type hints
  (`any` where absent) and docstrings rendered as doc comments, resolvable by
  `go get`

#### Scenario: publish clojars emits pure Clojure source
- **WHEN** a pure-Clojure library is published with `cljgo publish clojars`
- **THEN** its Clojure source is emitted in a layout a JVM Clojure `deps.edn`
  `:git/url`+`:sha` dependency can consume, with no compiled jar and no Java

### Requirement: publish clojars is gated on the transitive absence of Go interop

`publish clojars` MUST walk the library's whole transitive required surface and
refuse if any reachable form uses Go interop (`require-go`/ffi), naming the
offending `file:line`. The gate MUST be `uses-go-interop?`, NOT "no Java" — Java
interop MUST NOT disqualify a clojars artifact (it runs on the JVM). The walk
MUST reuse the existing `emit.CompileProgram` transitive-require traversal, not
introduce a new one.

#### Scenario: buried Go interop is caught and cited
- **WHEN** a namespace required two levels deep (`core→mid→leaf`) uses
  `require-go`, and `publish clojars` runs on `core`
- **THEN** publish fails naming the offending `file:line` (e.g. `leaf.clj:3`),
  even though the intermediate namespaces are themselves pure

#### Scenario: a pure library publishes to clojars with zero false positives
- **WHEN** a wholly pure-Clojure library (that may use Java interop) is published
  to clojars
- **THEN** it succeeds — Java does not disqualify it, and no pure form is
  wrongly flagged

#### Scenario: the same pure library also publishes to go
- **WHEN** a pure-Clojure library is published with both `publish go` and
  `publish clojars`
- **THEN** both succeed from the same source — a pure library is the only
  artifact that reaches both ecosystems

#### Scenario: a Go-using library is refused from clojars but allowed to go
- **WHEN** a library uses `require-go`/ffi
- **THEN** `publish clojars` fails at the offending `file:line`, while
  `publish go` succeeds

### Requirement: Go-interop taint is per-namespace and derived from host nodes

The taint classifier MUST mark a namespace as Go-interop-using based on the
presence of the analyzer host nodes (`OpHostRef`, `OpHostCall`, `OpHostMethod`,
`OpHostField`, `OpHostNew`) in its forms, computed once over the CompileProgram
traversal. The whole-library gate MUST be the OR (per-namespace gate the lookup)
over the resulting per-namespace map. A pluggable predicate slot MUST be reserved
for future `ffi`/`c-link` taint without changing the traversal.

#### Scenario: whole-library gate equals the AND of per-namespace purity
- **WHEN** the classifier runs over a library's reachable namespaces
- **THEN** the whole-library "pure" verdict equals AND(per-namespace pure over
  all reachable namespaces), both falling out of the one traversal

### Requirement: A Java-tainted namespace fails loud and per-namespace, never silently

When a namespace uses unsupported Java interop, requiring it MUST hard-error at
that point with `file:line` and a message that Java interop is unsupported on
cljgo's Go host. It MUST NOT return `nil`/`""` (the ADR 0053 unforgivable
divergence). Purity being per-namespace, the pure namespaces of the same
dependency MUST remain usable. A project MAY opt into strict resolve-time
rejection of any dependency declaring Java taint.

#### Scenario: requiring a Java-tainted namespace hard-errors
- **WHEN** a namespace using an unsupported Java form is required
- **THEN** it hard-errors naming `file:line`, exactly as loudly as an unlinked
  Go module — never `nil`

#### Scenario: pure siblings of a Java-tainted namespace stay usable
- **WHEN** a dependency contains both Java-tainted and pure namespaces
- **THEN** requiring a pure namespace succeeds; only the tainted one errors

### Requirement: The Java courtesy diagnostic is certain-only and never a gate

A `certain-java?` diagnostic MAY flag the self-identifying JVM surfaces
(`(System/…)`, `(Math/…)`, `import`, `new`, `java.*`, `clojure.java.*`) to
upgrade a raw downstream error to a named one. It MUST have zero false positives:
it MUST NOT flag valid Go/pure code, MUST NOT guess the undecidable bare
dot-form `(.method obj)`, and MUST NOT act as a publish or build gate.

#### Scenario: certain Java surfaces are named early
- **WHEN** code contains `(System/getProperty …)` or `(new java.util.UUID …)`
- **THEN** the diagnostic names it with `file:line`, but the build's
  success/failure is still decided by the compiler/interpreter, not this
  diagnostic

#### Scenario: ambiguous and value forms are not flagged
- **WHEN** code contains a bare `(.method obj)`, `(instance? String x)`,
  `(catch Exception e)`, or a bare `java.util.UUID` class-ref value
- **THEN** the diagnostic does NOT flag it (no false positive), leaving the
  decision to the downstream net

