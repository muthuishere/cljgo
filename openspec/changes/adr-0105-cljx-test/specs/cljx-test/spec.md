## ADDED Requirements

### Requirement: cljx.test provides mocks, spies and stubs with automatic restoration

`cljx.test` MUST provide `mock` (a recording callable, optionally with an
implementation or a configured return), the inspectors `calls`, `call-count`,
`called?`, `called-with?`, and `spy`/`stub` which wrap or replace an existing
var. A spy MUST forward to the real implementation while recording; a stub MUST
replace it. Both MUST be scoped so the original var is restored when the
scope exits, including when the body throws, and MUST NOT leak between tests.
Behaviour MUST be identical interpreted and compiled (spike s66).

#### Scenario: spy records and forwards

- GIVEN a var `fetch-user` with a real implementation
- WHEN it is spied and called twice inside the spy scope
- THEN `call-count` is 2, `called-with?` sees the arguments, the real
  implementation's results were returned, and after the scope the var is
  restored

### Requirement: cljx.test captures printed output by default

Every `cljx.test` test MUST capture `*out*` and `*err*` without the author
writing any binding boilerplate. `(printed)` MUST return the captured text and
`(printed? needle)` MUST accept a literal string or a regex. Captured output
MUST be replayed in the failure report when a test fails, and capture MUST
compose with mocks/stubs.

#### Scenario: assert on what the code printed

- GIVEN a function under test that calls `println`
- WHEN the test asserts `(printed? "rendering for")`
- THEN the assertion passes without the test binding `*out*` itself

### Requirement: a failing test suite fails the process on every path

The test runner MUST set a non-zero process exit code when any test fails —
interpreted (`cljgo test`) AND in a compiled test binary. `cljgo test
--compiled` MUST run the same tests through the AOT path.

#### Scenario: compiled failure is not silently green

- GIVEN a suite with one failing test
- WHEN it runs inside a compiled binary
- THEN the process exits non-zero
