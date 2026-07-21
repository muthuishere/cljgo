;; reductions (fundamentals audit 2026-07): lazy seq of intermediate
;; reduce values; no-init empty coll yields [(f)]; lazy over infinite seqs.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (reductions + [1 2 3 4]) => (1 3 6 10)
;;   (reductions + 10 [1 2 3]) => (10 11 13 16)
;;   (reductions + []) => (0)
;;   (reductions + 5 []) => (5)
;;   (reductions conj [] [1 2 3]) => ([] [1] [1 2] [1 2 3])
;;   (take 5 (reductions + (range))) => (0 1 3 6 10)
[(reductions + [1 2 3 4])
 (reductions + 10 [1 2 3])
 (reductions + [])
 (reductions + 5 [])
 (reductions conj [] [1 2 3])
 (take 5 (reductions + (range)))]
;; expect: [(1 3 6 10) (10 11 13 16) (0) (5) ([] [1] [1 2] [1 2 3]) (0 1 3 6 10)]
