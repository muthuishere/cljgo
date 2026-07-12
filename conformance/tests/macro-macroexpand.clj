;; macroexpand-1 / macroexpand as core fns (design/03 §4). Oracle
;; (clojure 1.12.5): (macroexpand '(when a b)) => (if a (do b)).
(defmacro unless [t e] (list 'if t nil e))
[(macroexpand-1 '(unless false 42)) (macroexpand '(when a b))]
;; expect: [(if false nil 42) (if a (do b))]
