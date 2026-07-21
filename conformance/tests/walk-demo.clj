;; clojure.walk postwalk-demo/prewalk-demo (fundamentals audit 2026-07):
;; print one "Walked: <form>" line per visited sub-form (post-order /
;; pre-order) and return the form. The printed lines are part of this
;; file's canonical output, so the dual harness pins them byte-identically.
;; oracle (clojure 1.12.5, 2026-07-21) prints, in order:
;;   Walked: 1 / Walked: 2 / Walked: [1 2] / Walked: 3 / Walked: [3]
;;   / Walked: [[1 2] [3]]   then (pre-order)
;;   Walked: [[1 2] [3]] / Walked: [1 2] / Walked: 1 / Walked: 2
;;   / Walked: [3] / Walked: 3
;; and both return the walked form.
(require '[clojure.walk :as w])
(def post-ret (w/postwalk-demo [[1 2] [3]]))
(def pre-ret (w/prewalk-demo [[1 2] [3]]))
[post-ret pre-ret]
;; expect: [[[1 2] [3]] [[1 2] [3]]]
