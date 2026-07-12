## ADDED Requirements

### Requirement: every diagnostic is a structured value with a stable code
The system SHALL construct every compiler diagnostic (reader, analyzer,
emitter, interop) as a structured value carrying a stable error code from the
banded registry (R/A/E/I), severity (error|warning|note), message, and full
location (file, line, column, end_line, end_column), with optional
expected/found, machine-applicable fixes[] (title, replacement, byte_range),
and related[] notes.

#### Scenario: analyzer error carries code and span
- **WHEN** source containing `(recur 1)` outside any loop or fn is checked
- **THEN** the diagnostic has an A-band error_code, severity "error", and a location whose span covers the recur form

### Requirement: human rendering is unchanged by default
The system SHALL render diagnostics as the existing human-readable Clojure-
style error text by default, byte-identical to pre-change output for
existing errors, with JSON produced only on request.

#### Scenario: default output stays stable
- **WHEN** a program with a known reader error is compiled without `--json`
- **THEN** stderr matches the frozen pre-change human error text exactly

### Requirement: --json emits the versioned diagnostic envelope on any verb
The system SHALL accept `--json` on every CLI verb and render all collected
diagnostics as a single `cljgo-diag/1` JSON envelope with snake_case keys and
UTF-8 byte offsets in fixes' byte_range, leaving exit codes unchanged.

#### Scenario: agent consumes a fix
- **WHEN** `cljgo build --json` runs on source with a fixable error
- **THEN** the output parses as JSON, the diagnostic includes fixes[] with a byte_range that, applied to the exact source bytes, produces the suggested replacement

### Requirement: the error-code registry is append-only and documented
The system SHALL keep the code registry as typed entries in source with a
generated committed lock snapshot and one explain page per code, and SHALL
fail the test suite if an existing code is removed, renumbered, or has its
locked summary altered, or if any code lacks an explain page.

#### Scenario: removing a code breaks CI
- **WHEN** a registered error code is deleted from the registry while the lock snapshot still lists it
- **THEN** the test suite fails identifying the missing code

#### Scenario: new code requires an explain page
- **WHEN** a new code is registered without a corresponding explain page
- **THEN** the test suite fails naming the undocumented code
