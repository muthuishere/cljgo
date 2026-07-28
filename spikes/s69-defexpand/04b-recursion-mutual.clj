;; s69 Q4b — mutual recursion, also isolated (also divergent).
(println "defining od/evn ...")
(defexpand od  [n] (if (zero? n) false (evn (- n 1))))
(defexpand evn [n] (if (zero? n) true  (od  (- n 1))))
(println "both definitions SUCCEEDED")
(println "calling (evn 4) ...")
(println "(evn 4) =>" (evn 4))
(println "returned — no divergence")
