;; for — list comprehension (v0: binding pairs, no modifiers). Lazy.
;; oracle (clojure 1.12.5):
;;   (for [x (range 3)] (* x x)) => (0 1 4)
;;   (for [x [1 2] y [:a :b]] [x y]) => ([1 :a] [1 :b] [2 :a] [2 :b])
[(for [x (range 3)] (* x x))
 (for [x [1 2] y [:a :b]] [x y])]
;; expect: [(0 1 4) ([1 :a] [1 :b] [2 :a] [2 :b])]
