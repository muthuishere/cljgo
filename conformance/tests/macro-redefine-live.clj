;; Re-defmacro takes effect on the very next form (design/03 §7a):
;; the analyzer consults the var's :macro flag per form.
(defmacro m [] 1)
(def a (m))
(defmacro m [] 2)
[(= a 1) (m)]
;; expect: [true 2]
