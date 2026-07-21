;; clojure.test's remaining public surface (fundamentals audit 2026-07):
;; the dynamic vars a runner rebinds, the with-test/deftest-/set-test
;; definition family, and the small predicates clojure.test exports.
;; oracle (clojure 1.12.5, 2026-07-21): the same forms printed
;;   [{:test 0, :pass 0, :fail 0, :error 0} true nil
;;    true false nil false
;;    true 42 true true true false]
;; Note (function? 'no-such-sym) => nil, not false: it is an `and` over a
;; failed resolve, and clojure.test never coerces the result.
(require '[clojure.test :as t])
(t/with-test (defn wt [] 42) (t/is (= 42 (wt))))
(t/deftest- ptest (t/is (= 1 1)))
[t/*initial-report-counters*
 t/*load-tests*
 t/*stack-trace-depth*
 (t/function? '+)
 (t/function? 'let)
 (t/function? 'no-such-sym)
 (t/function? 5)
 (some? (:test (meta #'wt)))
 (wt)
 (:private (meta #'ptest))
 (t/get-possibly-unbound-var #'t/*load-tests*)
 (t/successful? {:fail 0 :error 0})
 (t/successful? {:fail 1 :error 0})]
;; expect: [{:test 0, :pass 0, :fail 0, :error 0} true nil true false nil false true 42 true true true false]
