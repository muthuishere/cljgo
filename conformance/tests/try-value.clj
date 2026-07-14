;; try with no throw evaluates its body as an implicit do, yielding the
;; last expression's value. Oracle (clojure 1.12.5): (try 1 2 3) => 3.
(try 1 2 3)
;; expect: 3
