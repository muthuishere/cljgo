;; Multi-arity defmacro (methods get &form/&env prepended per arity).
(defmacro m ([x] x) ([x y] (list '+ x y)))
[(m 5) (m 2 3)]
;; expect: [5 5]
