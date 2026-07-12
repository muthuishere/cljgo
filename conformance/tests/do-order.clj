;; A top-level (do ...) is split and evaluated form-by-form, so earlier
;; defs are visible to later siblings (design/03 §6).
(do (def a 1)
    (def b 2)
    (+ a b))
;; expect: 3
