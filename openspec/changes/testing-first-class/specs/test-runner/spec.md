## ADDED Requirements

### Requirement: cljgo test discovers and runs tests with zero config
The system SHALL provide a `cljgo test` CLI verb that, with no configuration,
loads all project source and test paths and runs every var carrying `:test`
metadata, including `(deftest ...)` forms colocated with source code,
reporting pass/fail/error counts and exiting non-zero on any failure.

#### Scenario: colocated test is discovered
- **WHEN** a source namespace contains a fn and a `(deftest ...)` beside it and `cljgo test` runs with no config files
- **THEN** the colocated test executes and is included in the reported counts

#### Scenario: filter flags narrow the run
- **WHEN** `cljgo test --ns my.app.core-test` runs
- **THEN** only tests in that namespace execute

### Requirement: colocated tests are eliminated from production builds
The system SHALL exclude all `deftest` and `defbench` forms (and forms marked
`^:test-only`) from production build outputs by emitting them into Go
`_test.go` files, so `cljgo build` binaries and `--lib` outputs contain no
test code.

#### Scenario: binary carries no test code
- **WHEN** a namespace with colocated deftests is built with `cljgo build`
- **THEN** the emitted non-test Go files contain no deftest-derived code and the binary runs without the tests' side effects

### Requirement: dual-harness testing for user code
The system SHALL run the same discovered tests interpreted by default,
through the AOT path under `--compiled`, and under `--both` SHALL execute
both runs, compare per-test outcome, assertion counts, and normalized failure
messages, report each divergent test with both sides' results, and exit
non-zero if any divergence or failure occurred.

#### Scenario: divergence is reported and fails the run
- **WHEN** a test passes interpreted but fails compiled and `cljgo test --both` runs
- **THEN** the output contains a DIVERGE line naming the fully-qualified test with both outcomes and the exit code is non-zero

#### Scenario: agreement passes quietly
- **WHEN** all tests produce identical outcomes in both modes under `--both`
- **THEN** no DIVERGE lines are printed, a summary reports zero divergences, and the exit code is zero

### Requirement: benchmarks run as tests
The system SHALL provide `(defbench name & body)` registering a benchmark
that `cljgo test --bench` executes — via a timing harness interpreted, and
via emitted Go `testing.B` Benchmark functions when compiled — excluded from
normal test runs and from production builds.

#### Scenario: bench runs only under --bench
- **WHEN** a project defines a defbench and `cljgo test` runs without `--bench`
- **THEN** the benchmark does not execute; running with `--bench` executes it and reports timing
