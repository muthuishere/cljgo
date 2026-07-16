;; Batch 4 companions (ADR 0022): map's 4+-arity (n collections, stops at
;; the shortest, lazy on infinite input), take-nth's 1-arity transducer
;; (negative n behaves like positive, per JVM), and mapcat's lazy variadic
;; arity — (take 5 (mapcat f (range))) must terminate: cljgo's mapcat
;; flattens through an explicitly lazy -concat-seqs instead of JVM's
;; (apply concat xs), because cljgo's apply forces the whole last-arg seq.
;; Oracle (clojure 1.12.5):
;; [(111 222) (0 3 6 9 12) (0 3 6 9 12) ([1 3 5 7] [2 4 6 8]) (3 6)
;;  [0 2 4 6 8] [0 2 4 6 8] (0 0 1 1 2) (:a 1 :b 2 :c 3)]
[(map + [1 2] [10 20] [100 200])
 (map + (range 5) (range 5) (range 5))
 (doall (take 5 (map + (range) (range) (range))))
 (map vector [1 2] [3 4] [5 6] [7 8])
 (map + [1 2 3] [1 2] [1 2 3])
 (into [] (take-nth 2) (range 10))
 (into [] (take-nth -2) (range 10))
 (doall (take 5 (mapcat (fn [x] (repeat 2 x)) (range))))
 (mapcat identity {:a 1 :b 2 :c 3})]
;; expect: [(111 222) (0 3 6 9 12) (0 3 6 9 12) ([1 3 5 7] [2 4 6 8]) (3 6) [0 2 4 6 8] [0 2 4 6 8] (0 0 1 1 2) (:a 1 :b 2 :c 3)]
