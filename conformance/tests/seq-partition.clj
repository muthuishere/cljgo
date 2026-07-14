;; partition / partition-all (clojure.core).
;; oracle (clojure 1.12.5):
;;   (partition 2 (range 6)) => ((0 1) (2 3) (4 5))
;;   (partition-all 2 (range 5)) => ((0 1) (2 3) (4))
[(partition 2 (range 6))
 (partition-all 2 (range 5))]
;; expect: [((0 1) (2 3) (4 5)) ((0 1) (2 3) (4))]
