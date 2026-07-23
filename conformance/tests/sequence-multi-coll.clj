;; Batch A3 bug fix: (sequence xform coll & colls) — the multi-coll
;; transducer arity (the JVM's TransformerIterator.createMulti) used to
;; throw "wrong number of args (3) passed to: fn". Each step now pulls the
;; first of EVERY source into the xform's step as one multi-input call,
;; stopping at the shortest source; map's transducer gained the matching
;; ([result input & inputs]) step. Covers 2-coll, 3-coll, uneven lengths,
;; an infinite source bounded by a finite one, transducer early
;; termination (take), and the single-coll regression case.
;; Oracle (clojure 1.12.5): each element verified 2026-07-23.
[(sequence (map +) [1 2] [10 20])
 (sequence (map +) [1 2 3] [10 20] [100 200 300])
 (sequence (map vector) [1 2 3] [:a :b])
 (sequence (map +) (range) [1 2])
 (sequence (comp (map +) (take 2)) [1 2 3] [10 20 30])
 (sequence (map inc) [1 2 3])
 (sequence (map str) [1 2] [:a :b] ["x" "y"])]
;; expect: [(11 22) (111 222) ([1 :a] [2 :b]) (1 3) (11 22) (2 3 4) ("1:ax" "2:by")]
