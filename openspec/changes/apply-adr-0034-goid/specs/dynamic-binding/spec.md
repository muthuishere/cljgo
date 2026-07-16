## ADDED Requirements

### Requirement: cheap goroutine-ID lookup on the dynamic-binding hot path
The runtime's goroutine-ID lookup (`pkg/lang/internal/goid.Get`), invoked
on every dynamic-var deref and every thread-binding push/pop/clone, SHALL
NOT capture or parse a stack trace on supported configurations
(amd64/arm64 on the vetted Go toolchain range). It SHALL read the ID via
a getg()-based field access selected at compile time by build tags, with
the `runtime.Stack()` text-parse retained as the complete fallback for
all other configurations. The fast path and the fallback SHALL return
identical IDs for the same goroutine.

#### Scenario: identical IDs across concurrent goroutines
- **WHEN** hundreds of goroutines concurrently compare the fast lookup
  against the stack-parse lookup (under the race detector)
- **THEN** every comparison is equal and no race is reported

#### Scenario: wrong offset fails loudly, never silently
- **WHEN** the compiled-in `runtime.g` field offset does not match the
  running toolchain
- **THEN** the process panics at package init, before any dynamic
  binding can be keyed by a wrong ID

#### Scenario: binding semantics unchanged
- **WHEN** the existing binding-conveyance (futures/bound-fn) and nREPL
  session-isolation tests run with the fast lookup
- **THEN** all pass unchanged
