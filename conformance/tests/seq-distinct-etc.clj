;; distinct/interpose/interleave/flatten/reverse/concat/vec (clojure.core).
;; oracle (clojure 1.12.5):
;;   (distinct [1 1 2 3 3 3 4]) => (1 2 3 4)
;;   (interpose 0 [1 2 3]) => (1 0 2 0 3)
;;   (interleave [1 2 3] [:a :b :c]) => (1 :a 2 :b 3 :c)
;;   (flatten [1 [2 [3 [4]]]]) => (1 2 3 4)
;;   (reverse [1 2 3]) => (3 2 1)
;;   (concat [1 2] [3 4] [5]) => (1 2 3 4 5)
;;   (vec (list 1 2 3)) => [1 2 3]
[(distinct [1 1 2 3 3 3 4])
 (interpose 0 [1 2 3])
 (interleave [1 2 3] [:a :b :c])
 (flatten [1 [2 [3 [4]]]])
 (reverse [1 2 3])
 (concat [1 2] [3 4] [5])
 (vec (list 1 2 3))]
;; expect: [(1 2 3 4) (1 0 2 0 3) (1 :a 2 :b 3 :c) (1 2 3 4) (3 2 1) (1 2 3 4 5) [1 2 3]]
