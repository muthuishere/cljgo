;; cljx.test output capture (ADR 0105): capture is the DEFAULT inside a cljx
;; test — the body runs with *out* captured and `(printed)` / `(printed? …)`
;; read it with no binding boilerplate — and `(capturing …)` is the explicit
;; form for use outside a test. Capture COMPOSES with stubs, and a failing test
;; replays what it printed (that replay's text is frozen by
;; cljx-test-expect-failure-message.clj).
;;
;; The whole file runs the tests through clojure.test's own machinery (a cljx
;; test IS a clojure.test test) with the counters bound by hand, so nothing is
;; printed when everything passes and the frozen value is the assertion tally.
;;
;; oracle: skip — cljx.test is a cljgo namespace (ADR 0105's cljx.* developer
;; experience tier) with no JVM analog to freeze byte-for-byte. Frozen against
;; cljgo's own output; REPL-vs-binary parity is enforced by the dual harness.
(require '[cljx.test :as x])
(require '[clojure.test :as t])

(defn fetch-user [id] {:id id :name "REAL"})
(defn render [id]
  (let [u (fetch-user id)]
    (println "rendering for" (:name u))
    (str "Hello, " (:name u) "!")))

(x/describe "capture"
  (x/it "is on by default"
    (render 3)
    (x/expect (x/printed? "rendering for") :to-be true)
    (x/expect (x/printed? #"rendering \w+") :to-be true)
    (x/expect (x/printed) :to-contain "REAL"))

  (x/it "composes with a stub"
    (x/with-spies [db (x/stub #'fetch-user (fn [_] {:name "Quiet"}))]
      (x/expect (render 9) :to-be "Hello, Quiet!")
      (x/expect (x/printed? "rendering for Quiet") :to-be true))))

;; the explicit form, OUTSIDE any test
(def cap (x/capturing (render 3)))

(def tally
  (binding [t/*report-counters* (atom t/initial-report-counters)]
    (t/test-vars [#'it-capture-is-on-by-default #'it-capture-composes-with-a-stub])
    (deref t/*report-counters*)))

[tally
 (:value cap)
 (x/printed cap)
 (x/printed? cap "rendering for REAL")
 (x/printed? cap #"rendering for \w+")
 (x/printed? cap "not printed")]
;; expect: [{:test 2, :pass 5, :fail 0, :error 0} "Hello, REAL!" "rendering for REAL\n" true true false]
