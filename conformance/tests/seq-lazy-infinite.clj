;; Laziness: take over infinite producers (range/iterate/cycle/repeat/
;; repeatedly). Realizing only a prefix proves map/take/producers are lazy.
;; oracle (clojure 1.12.5):
;;   (take 3 (range)) => (0 1 2)
;;   (take 5 (iterate inc 0)) => (0 1 2 3 4)
;;   (take 7 (cycle [1 2 3])) => (1 2 3 1 2 3 1)
;;   (take 4 (repeat :y)) => (:y :y :y :y)
;;   (take 3 (repeatedly (fn [] 1))) => (1 1 1)
[(take 3 (range))
 (take 5 (iterate inc 0))
 (take 7 (cycle [1 2 3]))
 (take 4 (repeat :y))
 (take 3 (repeatedly (fn [] 1)))]
;; expect: [(0 1 2) (0 1 2 3 4) (1 2 3 1 2 3 1) (:y :y :y :y) (1 1 1)]
