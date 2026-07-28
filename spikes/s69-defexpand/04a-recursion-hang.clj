;; s69 Q4a — the DIVERGENT case, isolated because it does not terminate.
;; Run under `timeout`: see run-recursion.sh.
(println "defining a self-referential defexpand ...")
(defexpand fact [n] (if (<= n 1) 1 (* n (fact (- n 1)))))
(println "definition SUCCEEDED (the body is only captured, never checked)")
(println "now calling (fact 5) ...")
(println "(fact 5) =>" (fact 5))
(println "returned — no divergence")
