;; Re-def replaces the var's root, never its identity: a caller captured
;; BEFORE the re-def sees the new value (design/03 §7a).
(def f (fn* [x] (+ x 1)))
(def g (fn* [x] (f x)))
(def f (fn* [x] (* x 10)))
(g 5)
;; expect: 50
