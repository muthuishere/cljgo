;; M0 exit demo: recursive factorial via self-named fn* (design/00 §6).
(def fact (fn* fact [n] (if (< n 2) 1 (* n (fact (- n 1))))))
(fact 10)
;; expect: 3628800
