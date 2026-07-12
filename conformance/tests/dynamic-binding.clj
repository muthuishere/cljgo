;; ^:dynamic def + binding: the thread binding is visible through a fn
;; called inside the dynamic extent, and reverts after.
;; Oracle (Clojure 1.12, 2026-07-12):
;;   (def ^:dynamic *x* 1) (def probe (fn* [] *x*))
;;   (binding [*x* 2] (probe)) → 2, then (probe) → 1.
(def ^:dynamic *x* 1)
(def probe (fn* [] *x*))
[(binding [*x* 2] (probe)) (probe)]
;; expect: [2 1]
