;; Batch 4 transducers (ADR 0022, design/08 §5): transduce, completing,
;; and the reduced-box helpers unreduced/ensure-reduced. transduce applies
;; xform to f, reduces coll, then calls the completing (1-arity) of the
;; xformed rf exactly once. Oracle (clojure 1.12.5):
;; [9 109 0 [1 2 3] 3 30 5 5 true true]
[(transduce (map inc) + [1 2 3])
 (transduce (map inc) + 100 [1 2 3])
 (transduce (take 0) + (range 5))
 (transduce (comp (map inc) (take 3)) conj (range 10))
 ((completing +) 1 2)
 ((completing + (fn [x] (* x 10))) 3)
 (unreduced (reduced 5)) (unreduced 5)
 (reduced? (ensure-reduced 5)) (reduced? (ensure-reduced (reduced 5)))]
;; expect: [9 109 0 [1 2 3] 3 30 5 5 true true]
