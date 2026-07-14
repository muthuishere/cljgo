;; take/drop/take-while/drop-while/take-nth/split-at (clojure.core).
;; oracle (clojure 1.12.5):
;;   (take 3 (range 10)) => (0 1 2)
;;   (drop 2 [1 2 3 4]) => (3 4)
;;   (take-while #(< % 3) (range 10)) => (0 1 2)
;;   (drop-while #(< % 3) (range 10)) => (3 4 5 6 7 8 9)
;;   (take-nth 2 (range 10)) => (0 2 4 6 8)
;;   (split-at 2 [1 2 3 4 5]) => [(1 2) (3 4 5)]
[(take 3 (range 10))
 (drop 2 [1 2 3 4])
 (take-while (fn [x] (< x 3)) (range 10))
 (drop-while (fn [x] (< x 3)) (range 10))
 (take-nth 2 (range 10))
 (split-at 2 [1 2 3 4 5])]
;; expect: [(0 1 2) (3 4) (0 1 2) (3 4 5 6 7 8 9) (0 2 4 6 8) [(1 2) (3 4 5)]]
