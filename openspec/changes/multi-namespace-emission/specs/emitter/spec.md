## ADDED Requirements

### Requirement: One Go package per Clojure namespace
The system SHALL compile a program whose entry file requires other
file-backed namespaces into one generated Go module containing one Go
package per dependency namespace (directory = ns segments munged `.`→`/`,
`-`→`_`; package name = munged last segment) plus the existing `package
main` holding the entry namespace's forms and bootstrap. A program with no
file-backed requires SHALL emit exactly what the single-file path emits
today.

#### Scenario: three-namespace program builds and runs
- **WHEN** `cljgo build` compiles an entry file requiring `multi.util`,
  which requires `multi.data` (files resolved relative to the requiring
  file per ADR 0042 §4)
- **THEN** the generated module contains `multi/util/` and `multi/data/`
  packages plus `main.go`, `go build` succeeds, and the binary's stdout is
  byte-identical to the interpreted run of the same entry file

#### Scenario: single-file path unchanged
- **WHEN** a program with no file-backed requires is compiled through the
  program path
- **THEN** emission delegates to the existing single-file module writer and
  the output is unchanged

### Requirement: Registry-triggered dependency loading, exactly once
Each emitted dependency package SHALL register its guarded `Load()` in the
runtime lib-provider registry from package `init()`, and the replayed
`(require …)` form SHALL trigger that `Load()` at its source position, so
load-time side effects interleave exactly as in the interpreter (oracle:
Clojure 1.12.5) and each namespace loads at most once per process. Requiring
packages SHALL blank-import their file-backed requires so the linker keeps
them.

#### Scenario: side-effect order is byte-identical
- **WHEN** the entry file prints before and after a `require` whose target
  prints at load time
- **THEN** the compiled binary prints before / dep / after — the same order
  as the interpreter

#### Scenario: diamond dependency loads once
- **WHEN** the entry requires both `multi.util` and `multi.data`, and
  `multi.util` also requires `multi.data`
- **THEN** `multi.data`'s load-time effects run exactly once in both modes

### Requirement: Compile-time require cycle detection
The module compiler SHALL reject cyclic file-backed requires with an error
naming the cycle (`cyclic load dependency: a -> b -> a` shape).

#### Scenario: two-namespace cycle
- **WHEN** `multi.a` requires `multi.b` and `multi.b` requires `multi.a`
- **THEN** compilation fails with a cyclic-load-dependency error naming both
  namespaces

### Requirement: Cross-namespace var references resolve through global interns
Emitted code referencing a var interned by another namespace SHALL use the
same hoisted `lang.InternVarName` package-level intern used for local vars
(ADR 0042 §3): same `*lang.Var` object across packages, per-call `Get()`
cost identical to an intra-namespace reference, redefinition semantics
preserved.

#### Scenario: cross-ns value and macro use
- **WHEN** the entry reads `multi.util/offset` (itself computed from
  `multi.data/base`) and uses a macro defined in `multi.data`
- **THEN** the compiled result equals the interpreted result (macros having
  expanded at analysis time)

### Requirement: Interpreter loads file-backed requires identically
The interpreter (REPL driver / eval harness) SHALL load a missing required
namespace from the filesystem using the same resolver, so the dual harness
can compare both modes; the single-file AOT API (`CompileFile`) SHALL
instead fail with an error directing callers to the program compiler rather
than emit a binary missing namespaces.

#### Scenario: dual harness runs a multi-ns conformance file
- **WHEN** a conformance test file under `tests/` requires namespaces whose
  sources live under `tests/multi/`
- **THEN** both the eval harness and the compiled harness produce identical
  output matching the frozen oracle expectation
