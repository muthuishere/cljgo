;; M1 exit demo (design/00 §6): a user macro defined with defmacro is
;; usable on the very next form. Oracle (clojure 1.12.5): 42.
(defmacro unless [t e] (list 'if t nil e))
(unless false 42)
;; expect: 42
