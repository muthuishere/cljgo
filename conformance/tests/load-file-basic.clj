;; load-file (issue #167): sequentially reads and evaluates a file given
;; an explicit path (relative to the process cwd, exactly like the JVM),
;; returning the value of the LAST top-level form and leaving any defs it
;; makes in the CURRENT namespace.
;; harness: eval -- load-file needs the reader and analyzer, so an
;; AOT-compiled binary panics with a named message instead (is not
;; available in an AOT-compiled binary, ADR 0046, same bucket as
;; eval/load-string); TestLoadFileAOTStub in pkg/corelib/aot_stubs_test.go
;; covers that leg with a Go regression test.
;; oracle (clojure 1.12.5, 2026-07-31, run from conformance/ via
;; clojure -M on an equivalent script): [true 42 41]
(def resolves? (some? (resolve (quote clojure.core/load-file))))
(def load-result (load-file "tests/conf/load-file-fixture.clj"))
[resolves? load-result lf-fixture-x]
;; expect: [true 42 41]
