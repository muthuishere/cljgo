;; Batch 4 transducers (ADR 0022, design/08 §5): the 1-arity
;; (transducer-returning) forms of the seq fns, driven through the new
;; (into to xform from) arity. Existing collection arities are untouched
;; (the precedence principle). Stateful xforms (take/drop/drop-while/
;; map-indexed/keep-indexed/distinct/interpose/partition-by/dedupe) hold
;; their state in an atom per xform application — single-use, like the
;; JVM's volatiles. Oracle (clojure 1.12.5):
;; [[2 3 4] [0 2 4 6 8] [1 3 5 7 9] [0 1 2] [3 4 5] [2 4 6] [1 8] [2 4]
;;  [[0 :a] [1 :b] [2 :c]] [:a :c] [1 2 3 4] [1 1 2 2 3 3] [1 2 3]
;;  [1 0 2 0 3] [[1 1] [2 2] [3 3]] [1 2 1] [1 2 3] [:one 2 :one]
;;  (4 3 2) {1 2, 3 4}]
[(into [] (map inc) [1 2 3])
 (into [] (filter even?) (range 10))
 (into [] (remove even?) (range 10))
 (into [] (take 3) (range))
 (into [] (drop 2) [1 2 3 4 5])
 (into [] (take-while even?) [2 4 6 1 8])
 (into [] (drop-while even?) [2 4 6 1 8])
 (into [] (keep (fn [x] (when (even? x) x))) [1 2 3 4])
 (into [] (map-indexed vector) [:a :b :c])
 (into [] (keep-indexed (fn [i x] (when (even? i) x))) [:a :b :c :d])
 (into [] cat [[1 2] [3 4]])
 (into [] (mapcat (fn [x] [x x])) [1 2 3])
 (into [] (distinct) [1 1 2 3 3 2])
 (into [] (interpose 0) [1 2 3])
 (into [] (partition-by even?) [1 1 2 2 3 3])
 (into [] (dedupe) [1 1 2 2 1 1])
 (into [] (random-sample 1.0) [1 2 3])
 (into [] (replace {1 :one}) [1 2 1])
 (into () (map inc) [1 2 3])
 (into {} (map identity) [[1 2] [3 4]])]
;; expect: [[2 3 4] [0 2 4 6 8] [1 3 5 7 9] [0 1 2] [3 4 5] [2 4 6] [1 8] [2 4] [[0 :a] [1 :b] [2 :c]] [:a :c] [1 2 3 4] [1 1 2 2 3 3] [1 2 3] [1 0 2 0 3] [[1 1] [2 2] [3 3]] [1 2 1] [1 2 3] [:one 2 :one] (4 3 2) {1 2, 3 4}]
