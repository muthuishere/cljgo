;; mapcat/keep/mapv/filterv and multi-coll map (clojure.core).
;; oracle (clojure 1.12.5):
;;   (mapcat (fn [x] [x x]) [1 2 3]) => (1 1 2 2 3 3)
;;   (keep (fn [x] (when (even? x) x)) (range 6)) => (0 2 4)
;;   (mapv inc [1 2 3]) => [2 3 4]
;;   (filterv even? (range 10)) => [0 2 4 6 8]
;;   (map + [1 2 3] [10 20 30]) => (11 22 33)
[(mapcat (fn [x] [x x]) [1 2 3])
 (keep (fn [x] (when (even? x) x)) (range 6))
 (mapv inc [1 2 3])
 (filterv even? (range 10))
 (map + [1 2 3] [10 20 30])]
;; expect: [(1 1 2 2 3 3) (0 2 4) [2 3 4] [0 2 4 6 8] (11 22 33)]
