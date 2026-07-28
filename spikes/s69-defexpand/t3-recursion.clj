;; s69 t3 — a self-referential defexpand. The body names the fn itself, so
;; expanding a call site produces another call site. Does it terminate?
(defexpand cd [n] (if (zero? n) :done (cd (dec n))))
(println "defined ok")
(println (cd 3))
