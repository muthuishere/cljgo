;; clojure.test failure REPORT SHAPE (ADR 0105 task 2.3). JVM clojure.test
;; 1.12.5 renders the banner as
;;   FAIL in (t-fail) (core_test.clj:9)
;; — the test NAMES from *testing-vars*, then the source position. cljgo used
;; to pr-str the var itself, leaking the reader form `#=(var user/t-fail)`
;; into every failure report, and carried no position at all. It now prints
;; the names like the JVM and appends {:file :line} — taken from the reader's
;; metadata on the `is` form (see *assertion-position*) rather than from a
;; stack frame, which the Go host has no way to walk.
;;
;; The assertions below are shape-only on purpose: the absolute path in the
;; banner is whatever path the harness handed the reader, so freezing the
;; whole line would freeze a temp directory.
;; harness: eval — clojure.test is interpreted (ADR 0012); a deliberately
;; failing suite also makes a compiled binary exit non-zero by design.
;; oracle: JVM Clojure 1.12.5 clojure.test/testing-vars-str — verified with
;; the `clojure` CLI: a failing (deftest t-fail (is (= 1 2))) prints
;; "FAIL in (t-fail) (<file>:<line>)".
(clojure.core/require 'clojure.test 'clojure.string)
(clojure.core/refer 'clojure.test)
(deftest t-fail
  (is (= 1 2)))
(let [s (with-out-str (run-tests 'user))]
  [(clojure.string/includes? s "FAIL in (t-fail)")
   (clojure.string/includes? s "#=(var")
   (clojure.string/includes? s "clojure-test-fail-report.clj:22")])
;; expect: [true false true]
