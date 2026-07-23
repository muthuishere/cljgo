;; Batch A3: the 1.11 vector-returning partition variants — partitionv
;; (n / n-step / n-step-pad, groups are vectors, incomplete unpadded
;; groups dropped, pad fills only to n), partitionv-all (keeps the short
;; tail; 1-arity is the vector-emitting transducer), and splitv-at
;; (vector head + seq tail).
;; Oracle (clojure 1.12.5): verified 2026-07-23.
[(partitionv 2 (range 5))
 (partitionv 2 3 (range 10))
 (partitionv 3 3 [:pad] (range 7))
 (partitionv-all 2 (range 5))
 (partitionv-all 2 3 (range 10))
 (into [] (partitionv-all 3) (range 7))
 (splitv-at 2 (range 5))
 (splitv-at 0 [1 2])
 (splitv-at 9 [1 2])
 (vector? (first (partitionv 2 [1 2 3 4])))
 (vector? (first (splitv-at 2 (range 5))))]
;; expect: [([0 1] [2 3]) ([0 1] [3 4] [6 7]) ([0 1 2] [3 4 5] [6 :pad]) ([0 1] [2 3] [4]) ([0 1] [3 4] [6 7] [9]) [[0 1 2] [3 4 5] [6]] [[0 1] (2 3 4)] [[] (1 2)] [[1 2] ()] true true]
