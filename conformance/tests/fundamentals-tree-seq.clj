;; tree-seq (fundamentals audit 2026-07): lazy depth-first walk of a tree.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (tree-seq seq? identity '((1 2 (3)) (4)))
;;     => (((1 2 (3)) (4)) (1 2 (3)) 1 2 (3) 3 (4) 4)
;;   (tree-seq vector? seq [1 [2 3] [4 [5]]])
;;     => ([1 [2 3] [4 [5]]] 1 [2 3] 2 3 [4 [5]] 4 [5] 5)
;;   (tree-seq map? vals {:a {:b 1}}) => ({:a {:b 1}} {:b 1} 1)
[(tree-seq seq? identity '((1 2 (3)) (4)))
 (tree-seq vector? seq [1 [2 3] [4 [5]]])
 (tree-seq map? vals {:a {:b 1}})]
;; expect: [(((1 2 (3)) (4)) (1 2 (3)) 1 2 (3) 3 (4) 4) ([1 [2 3] [4 [5]]] 1 [2 3] 2 3 [4 [5]] 4 [5] 5) ({:a {:b 1}} {:b 1} 1)]
