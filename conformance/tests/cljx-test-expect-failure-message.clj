;; cljx.test failure REPORT shape (ADR 0105 + the error doctrine): a failing
;; `expect` must name the check and state expected-vs-found. Each matcher
;; expands to an `is` form whose predicate NAMES the matcher and whose
;; arguments are the actual and expected VALUES, so clojure.test's own reporter
;; prints, with no custom reporter:
;;
;;   expected: (cljx.test/to-contain? ["ravi"] "asha")
;;     actual: (not (cljx.test/to-contain? ["ravi"] "asha"))
;;
;; and a :to-throw miss reads as a value comparison rather than a nil blow-up
;; ("<no exception thrown>" vs the needle).
;;
;; This file also freezes the CAPTURE REPLAY: the test printed a line, so the
;; captured output is replayed under the failures — while the FAIL lines
;; themselves are NOT swallowed by the capture (cljx.test routes the installed
;; reporter past the capture buffer to the runner's real stream).
;;
;; oracle: skip — cljx.test is a cljgo namespace (ADR 0105's cljx.* developer
;; experience tier) with no JVM analog to freeze byte-for-byte. Frozen against
;; cljgo's own output; REPL-vs-binary parity is enforced by the dual harness —
;; and error text reading the same in the REPL and in a binary is precisely the
;; doctrine's bar.
(require '[cljx.test :as x])
(require '[clojure.test :as t])
(require '[clojure.string :as str])

(x/it "fails loudly"
  (println "side effect line")
  (x/expect (+ 1 2) :to-be 4)
  (x/expect ["ravi"] :to-contain "asha")
  (x/expect "abc" :to-match #"zz")
  (x/expect (+ 1 1) :to-throw "boom")
  (x/expect 1 :to-be-nil))

(def report
  (with-out-str
    (binding [t/*report-counters* (atom t/initial-report-counters)]
      (t/test-vars [#'it-fails-loudly]))))

;; This file DELIBERATELY runs failing assertions — it exists to freeze the
;; failure-report TEXT, not to report a failing suite. Those failures still
;; bump clojure.test's PROCESS-level tally (`-process-failures`), and since
;; ADR 0105 task 2.1 a non-zero tally exits the process 1 in BOTH legs. That
;; is correct and must stay: a red suite has to fail CI. So this file clears
;; the tally it intentionally dirtied, otherwise the compiled leg exits 1 and
;; the dual harness reads a deliberate fixture as a real failure.
;; `*report-counters*` above is per-run and does NOT cover this — the process
;; tally is a separate atom bumped in do-report, the one choke point.
(reset! t/-process-failures 0)

;; the report is compared line by line so the frozen value stays readable
(str/split-lines report)
;; expect: ["" "FAIL in (it-fails-loudly) fails loudly " "expected: (clojure.core/= (+ 1 2) 4)" "  actual: (not (clojure.core/= 3 4))" "" "FAIL in (it-fails-loudly) fails loudly " "expected: (cljx.test/to-contain? [\"ravi\"] \"asha\")" "  actual: (not (cljx.test/to-contain? [\"ravi\"] \"asha\"))" "" "FAIL in (it-fails-loudly) fails loudly " "expected: (cljx.test/to-match? \"abc\" #\"zz\")" "  actual: (not (cljx.test/to-match? \"abc\" #\"zz\"))" "" "FAIL in (it-fails-loudly) fails loudly " "expected: (cljx.test/message-matches? (cljx.test/caught-message (clojure.core/fn [] (+ 1 1))) \"boom\")" "  actual: (not (cljx.test/message-matches? \"<no exception thrown>\" \"boom\"))" "" "FAIL in (it-fails-loudly) fails loudly " "expected: (clojure.core/nil? 1)" "  actual: (not (clojure.core/nil? 1))" ";; captured output — fails loudly" "side effect line" ";; end captured output"]
