;; as->: thread into a named binding at any position. Oracle (clojure 1.12.5):
;; [(as-> 5 x (+ x 1) (* x 2)) (as-> [1 2 3] v (conj v 4) (count v)) (as-> 0 n)]
;; => [12 4 0]
[(as-> 5 x (+ x 1) (* x 2))
 (as-> [1 2 3] v (conj v 4) (count v))
 (as-> 0 n)]
;; expect: [12 4 0]
