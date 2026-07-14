;; Calling a fn with an unsupported argument count is an arity error.
(def f (fn* f [x y] (+ x y)))
(f 1)
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; expect-error: wrong number of args (1) passed to: f
