;; Calling a fn with an unsupported argument count is an arity error.
(def f (fn* f [x y] (+ x y)))
(f 1)
;; expect-error: wrong number of args (1) passed to: f
