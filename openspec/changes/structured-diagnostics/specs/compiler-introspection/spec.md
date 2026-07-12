## ADDED Requirements

### Requirement: introspection is debug-gated
The system SHALL expose the compiler introspection API only under
`cljgo debug` (debug REPL) or `cljgo debug --stdio`; it SHALL NOT be linked
into emitted production binaries nor exposed by the default REPL.

#### Scenario: default surfaces stay clean
- **WHEN** the default REPL evaluates `(clojure.compiler/check "src")` and an emitted binary is inspected for the introspection symbols
- **THEN** the REPL reports the namespace unavailable and the binary contains no introspection endpoints

### Requirement: one endpoint surface over two transports
The system SHALL expose the same endpoint set — check, explain, suggest_fix,
get_ast, get_symbols, get_types, get_control_flow, get_data_flow — both as
fns in a clojure.compiler namespace returning Clojure data and as
newline-delimited JSON request/response methods over stdio, with identical
semantics and the cljgo-diag/1 schema for all diagnostics.

#### Scenario: transports agree
- **WHEN** the same erroneous source is passed to `(clojure.compiler/check ...)` in the debug REPL and to a `check` request over `cljgo debug --stdio`
- **THEN** both return the same diagnostics (same codes, locations, fixes) modulo Clojure-data vs JSON encoding

### Requirement: check, explain, suggest_fix, get_ast, and get_symbols ship functional
The system SHALL implement functionally at first release: check (source →
diagnostics), explain (error code → long-form explain page content),
suggest_fix (diagnostic id → its fixes[]), get_ast (form or file → the
analyzed AST as data with positions), and get_symbols (namespaces and vars
in scope).

#### Scenario: check-explain-fix loop
- **WHEN** an agent calls check on broken source, then explain with the returned error_code, then suggest_fix with the diagnostic id
- **THEN** check returns positioned diagnostics, explain returns the code's documentation, and suggest_fix returns the machine-applicable fixes for that diagnostic

#### Scenario: AST as data
- **WHEN** get_ast is called on `(if true 1 2)`
- **THEN** the response is a data structure whose root node identifies the if operation and carries source positions

### Requirement: unimplemented endpoints answer structurally
The system SHALL make get_types, get_control_flow, and get_data_flow respond
with a structured not-yet-available answer (a D-band coded diagnostic value),
never a transport error, an exception, or free text, until each is
implemented.

#### Scenario: stub is machine-consumable
- **WHEN** get_control_flow is called before its implementation lands
- **THEN** the response parses in the standard schema and carries the not-yet-available code rather than an error crash
