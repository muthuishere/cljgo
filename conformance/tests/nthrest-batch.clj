;; nthrest (design/08 batch E, ADR 0022): like drop, but returns coll
;; unchanged for n <= 0 and () (not nil) once exhausted — the () vs nil
;; distinction `=` respects.
;; oracle (clojure 1.12.5): confirmed interactively via `clojure -e`.
[(nthrest (range 0 10) 3)
 (nthrest [0 1 2 3 4 5] 3)
 (nthrest (range 0 10) 10)
 (nthrest (range 0 10) 100)
 (nthrest (range 3) -1)
 (nthrest nil 0)
 (nthrest nil 100)]
;; expect: [(3 4 5 6 7 8 9) (3 4 5) () () (0 1 2) nil ()]
