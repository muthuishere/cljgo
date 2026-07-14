;; range (1/2/3-arg), repeat (2-arg), list* (clojure.core).
;; oracle (clojure 1.12.5):
;;   (range 5) => (0 1 2 3 4)
;;   (range 2 8) => (2 3 4 5 6 7)
;;   (range 0 10 2) => (0 2 4 6 8)
;;   (repeat 3 :x) => (:x :x :x)
;;   (list* 1 2 [3 4]) => (1 2 3 4)
[(range 5)
 (range 2 8)
 (range 0 10 2)
 (repeat 3 :x)
 (list* 1 2 [3 4])]
;; expect: [(0 1 2 3 4) (2 3 4 5 6 7) (0 2 4 6 8) (:x :x :x) (1 2 3 4)]
