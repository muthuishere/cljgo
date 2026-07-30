;; cljx.test lifecycle hooks + time control (ADR 0105).
;;
;; before-each / after-each / before-all / after-all are use-fixtures sugar and
;; ACCUMULATE — clojure.test/use-fixtures replaces the namespace's fixture
;; list, so each hook read-appends-writes rather than clobbering the previous
;; one. Ordering for two tests with one hook of each kind is frozen below.
;;
;; with-frozen-time pins cljg.date's clocks (now in epoch millis, nano-time in
;; step with it) for the duration of its body; advance! moves them forward.
;;
;; oracle: skip — cljx.test is a cljgo namespace (ADR 0105's cljx.* developer
;; experience tier) with no JVM analog to freeze byte-for-byte. Frozen against
;; cljgo's own output; REPL-vs-binary parity is enforced by the dual harness.
(require '[cljx.test :as x])
(require '[clojure.test :as t])
(require '[cljg.date :as date])

(def trail (atom []))
(defn log! [k] (swap! trail conj k))

(x/before-all (fn [] (log! :before-all)))
(x/before-each (fn [] (log! :before-each)))
(x/after-each (fn [] (log! :after-each)))
(x/after-all (fn [] (log! :after-all)))

(x/it "one" (log! :one) (x/expect 1 :to-be 1))
(x/it "two" (log! :two) (x/expect 2 :to-be 2))

(x/it "controls time"
  (x/with-frozen-time 1000
    (x/expect (date/now) :to-be 1000)
    (x/expect (date/nano-time) :to-be 1000000000)
    (x/advance! 500)
    (x/expect (date/now) :to-be 1500)
    (x/expect (date/since 1000000000 (date/nano-time)) :to-be 500000000))
  ;; unfrozen again outside the body
  (x/expect (> (date/now) 1500) :to-be true))

;; advance! outside a frozen scope is a named error, not a nil blow-up
(def stray-advance
  (try (x/advance! 1) :no-error (catch Throwable e (ex-message e))))

(def tally
  (binding [t/*report-counters* (atom t/initial-report-counters)]
    (t/test-vars [#'it-one #'it-two #'it-controls-time])
    (deref t/*report-counters*)))

[tally (deref trail) stray-advance]
;; expect: [{:test 3, :pass 7, :fail 0, :error 0} [:before-all :before-each :one :after-each :before-each :two :after-each :before-each :after-each :after-all] "cljx.test/advance!: no frozen clock in scope (expected: a call inside (with-frozen-time t …); found: top level)."]
