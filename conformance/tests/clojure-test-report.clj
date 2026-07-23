;; clojure.test/report — THE reporter extension point (fundamentals batch
;; A2): do-report routes every event through the `report` multimethod
;; (dispatching on :type), exactly like upstream clojure.test, so a custom
;; reporter is one defmethod away. Here a NEW event type is registered (the
;; built-in :pass/:fail/:error/:summary methods are left untouched so the
;; long-standing frozen output of every other clojure-test conformance file
;; is unchanged — that unchanged-output property is itself half the point
;; of this test: the summary map below is byte-identical to the pre-report
;; wiring); an unknown event type hits the :default no-op method.
;; harness: eval — clojure.test is interpreted (ADR 0012); no compiled harness
;; oracle: skip — cljgo's clojure.test bootstrap slice; verified against JVM
;; Clojure 1.12.5 (`clojure` CLI, 2026-07-23): the same forms printed
;; [[:hello] {:test 1, :pass 1, :fail 0, :error 0, :type :summary} 1]
;; (JVM adds its "Testing user" banner on stdout; only the value is frozen).
(clojure.core/require 'clojure.test)
(clojure.core/refer 'clojure.test)
(def events (atom []))
(defmethod report ::event [m]
  (swap! events conj (:payload m)))
(deftest t-pass (is (= 1 1)))
(def summary (run-tests 'user))
(do-report {:type ::event :payload :hello})
(do-report {:type ::unregistered-goes-to-default})
[(deref events) summary (:test summary)]
;; expect: [[:hello] {:test 1, :pass 1, :fail 0, :error 0, :type :summary} 1]
