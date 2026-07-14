;; cond-> / cond->>: thread through a step only when its test is truthy;
;; init + each step evaluated once. Oracle (clojure 1.12.5):
;; [(cond-> 1 true inc false (* 100) true (* 2))
;;  (cond->> 1 true inc true (- 10))
;;  (cond-> {} true (assoc :a 1) false (assoc :b 2))] => [4 8 {:a 1}]
[(cond-> 1 true inc false (* 100) true (* 2))
 (cond->> 1 true inc true (- 10))
 (cond-> {} true (assoc :a 1) false (assoc :b 2))]
;; expect: [4 8 {:a 1}]
