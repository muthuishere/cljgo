;; pcalls (parallel thunk calls) and pvalues (parallel exprs), both over
;; pmap — ordered, deterministic results.
;; oracle (Clojure 1.12.5 CLI, 2026-07-23):
;; [(pcalls (fn [] 1) (fn [] 2) (fn [] 3)) (pvalues (+ 1 2) (* 3 4))]
;; => [(1 2 3) (3 12)]
[(pcalls (fn [] 1) (fn [] 2) (fn [] 3)) (pvalues (+ 1 2) (* 3 4))]
;; expect: [(1 2 3) (3 12)]
