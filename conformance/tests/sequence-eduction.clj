;; Batch 4 transducers (ADR 0022, design/08 §5): the lazy pull side —
;; (sequence xform coll) pulls source elements one at a time (works on an
;; infinite range under take), eduction composes its xforms and delegates,
;; and the new collection fns' seq arities (replace on vector vs list,
;; map-indexed, keep-indexed, partition-by, dedupe) match the JVM.
;; Oracle (clojure 1.12.5):
;; [(0 1) (1 2 3 4) (1 2 3) ([1 1] [2 2] [3 3]) (1 2 3) () (2 4 6) [2 3 4]
;;  [:a 1 :c] (1 :b 1) ([0 :a] [1 :b] [2 :c]) (:a :c) ((1 1) (2 2) (3 3))
;;  (1 2 1)]
[(sequence (take 2) (range 9))
 (doall (take 4 (sequence (map inc) (range))))
 (sequence (comp (map inc) (take 3)) (range))
 (sequence (partition-by even?) [1 1 2 2 3 3])
 (sequence [1 2 3]) (sequence nil)
 (seq (eduction (map inc) (filter even?) [1 2 3 4 5]))
 (into [] (eduction (map inc) [1 2 3]))
 (replace {0 :a 2 :c} [0 1 2])
 (replace {:a 1} (list :a :b :a))
 (map-indexed (fn [i x] [i x]) [:a :b :c])
 (keep-indexed (fn [i x] (when (even? i) x)) [:a :b :c :d])
 (partition-by even? [1 1 2 2 3 3])
 (dedupe [1 1 2 2 1 1])]
;; expect: [(0 1) (1 2 3 4) (1 2 3) ([1 1] [2 2] [3 3]) (1 2 3) () (2 4 6) [2 3 4] [:a 1 :c] (1 :b 1) ([0 :a] [1 :b] [2 :c]) (:a :c) ((1 1) (2 2) (3 3)) (1 2 1)]
