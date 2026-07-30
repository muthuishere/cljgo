;; cljx.test/expect (ADR 0105): the Bun-flavoured matcher family, each one a
;; thin delegation to clojure.test/is so a cljx assertion is reported by the
;; same runner as a plain `is`. This freezes the passing side of every matcher
;; and its :not-to-… negation; the FAILURE text is frozen separately by
;; cljx-test-expect-failure-message.clj.
;;
;; An UNKNOWN matcher is rejected at macroexpansion — "macroexpanding expect:
;; cljx.test/expect: unknown matcher :to-be-awesome (expected one of: …)" —
;; and so is not frozen here: it is a compile-time error, and an AOT binary has
;; no analyzer to raise it (ADR 0046), which would make this file eval-only.
;;
;; oracle: skip — cljx.test is a cljgo namespace (ADR 0105's cljx.* developer
;; experience tier) with no JVM analog to freeze byte-for-byte. Frozen against
;; cljgo's own output; REPL-vs-binary parity is enforced by the dual harness.
(require '[cljx.test :as x])
(require '[clojure.test :as t])

(x/describe "matchers"
  (x/it "compare values"
    (x/expect (+ 1 2) :to-be 3)
    (x/expect (+ 1 2) :not-to-be 4)
    (x/expect {:a 1} :to-equal {:a 1})
    (x/expect {:a 1} :not-to-equal {:a 2}))

  (x/it "test membership"
    (x/expect [1 2 3] :to-contain 2)
    (x/expect [1 2 3] :not-to-contain 9)
    (x/expect {:a 1} :to-contain :a)
    (x/expect #{:a} :to-contain :a)
    (x/expect "hello" :to-contain "ell")
    (x/expect "hello" :not-to-contain "zz"))

  (x/it "match strings and nil"
    (x/expect "abc" :to-match #"b")
    (x/expect "abc" :not-to-match #"zz")
    (x/expect nil :to-be-nil)
    (x/expect 0 :not-to-be-nil))

  (x/it "test throwing"
    (x/expect (throw (ex-info "access denied" {})) :to-throw)
    (x/expect (throw (ex-info "access denied" {})) :to-throw "denied")
    (x/expect (throw (ex-info "access denied" {})) :to-throw #"acc\w+")
    (x/expect (+ 1 1) :not-to-throw)))

(def tally
  (binding [t/*report-counters* (atom t/initial-report-counters)]
    (t/test-vars [#'it-matchers-compare-values
                  #'it-matchers-test-membership
                  #'it-matchers-match-strings-and-nil
                  #'it-matchers-test-throwing])
    (deref t/*report-counters*)))

tally
;; expect: {:test 4, :pass 18, :fail 0, :error 0}
