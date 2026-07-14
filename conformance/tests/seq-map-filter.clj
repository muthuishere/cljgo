;; Core HOFs: lazy map/filter/remove over a range (clojure.core).
;; oracle (clojure 1.12.5):
;;   (map inc [1 2 3]) => (2 3 4)
;;   (filter even? (range 10)) => (0 2 4 6 8)
;;   (remove even? (range 10)) => (1 3 5 7 9)
[(map inc [1 2 3])
 (filter even? (range 10))
 (remove even? (range 10))]
;; expect: [(2 3 4) (0 2 4 6 8) (1 3 5 7 9)]
