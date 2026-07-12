## ADDED Requirements

### Requirement: clojure.test core API compatibility
The system SHALL provide `deftest`, `is`, `testing`, `use-fixtures`
(:each and :once), `run-tests`, `run-all-tests`, and `successful?` with
behavior matching JVM Clojure 1.12.5 clojure.test for the supported surface,
so existing Clojure test code ports unchanged; each supported behavior SHALL
be covered by a conformance file with oracle-verified expectations.

#### Scenario: standard idioms port unchanged
- **WHEN** a test file using `(deftest t (testing "ctx" (is (= 1 1)) (is (thrown? Exception (throw (ex-info "x" {}))))))` runs under cljgo
- **THEN** the test passes with two passing assertions, matching JVM clojure.test outcomes

#### Scenario: fixtures wrap in clojure.test order
- **WHEN** a namespace registers :once and :each fixtures and runs two tests
- **THEN** the :once fixture runs exactly once around the namespace run and the :each fixture runs around each test, matching JVM ordering

### Requirement: is reports structured failure data
The system SHALL make failing `is` assertions report the expected form, the
actual value, and the source position, in the clojure.test report-map shape
(:type :fail with :expected/:actual), counted in the run summary.

#### Scenario: failure carries expected and actual
- **WHEN** `(is (= 1 2))` runs inside a deftest
- **THEN** the failure report includes expected `(= 1 2)`, an actual value showing the mismatch, and the test file line, and the summary counts one failure
