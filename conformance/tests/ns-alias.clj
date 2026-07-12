;; alias maps a shorthand to another namespace in the current one;
;; alias-qualified symbols resolve through it.
;; Oracle (Clojure 1.12, 2026-07-12): (alias 'o 'scratch) then o/y
;;   derefs scratch's var.
(in-ns 'm1.aliased)
(clojure.core/refer 'clojure.core)
(def v 5)
(in-ns 'user)
(alias 'al 'm1.aliased)
al/v
;; expect: 5
