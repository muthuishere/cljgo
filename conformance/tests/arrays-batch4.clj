;; Batch 4 arrays (ADR 0022, ADR 0025, design/08 §5). A cljgo array is a
;; native Go slice; aset mutates in place, aclone makes an independent copy.
;; Oracle (clojure 1.12.5): (seq (int-array 3)) => (0 0 0); (seq
;; (object-array 3)) => (nil nil nil); (vec (to-array [1 2 3])) => [1 2 3];
;; (seqable? (object-array 3)) => true; (associative? (to-array [1 2 3]))
;; => false; (coll? (to-array [1 2 3])) => false.
[(seq (int-array 3))
 (alength (int-array 3))
 (seq (int-array [1 2 3]))
 (seq (int-array (range 5)))
 (seq (object-array 3))
 (seq (to-array [1 2 3]))
 (vec (to-array [1 2 3]))
 (let [a (int-array 3)] (aset a 0 5) [(seq a) (aget a 0)])
 (seq (long-array [1 2 3]))
 (seq (double-array [1.0 2 3]))
 (seq (boolean-array [true false true]))
 (seq (char-array [\a \b \c]))
 (let [a (int-array [1 2 3]) b (aclone a)] (aset b 0 99) [(seq a) (seq b)])
 (seq (into-array [1 2 3]))
 (seqable? (object-array 3))
 (associative? (to-array [1 2 3]))
 (coll? (to-array [1 2 3]))
 (= (vec (int-array [1 2 3])) [1 2 3])]
;; expect: [(0 0 0) 3 (1 2 3) (0 1 2 3 4) (nil nil nil) (1 2 3) [1 2 3] [(5 0 0) 5] (1 2 3) (1.0 2.0 3.0) (true false true) (\a \b \c) [(1 2 3) (99 2 3)] (1 2 3) true false false true]
