;; set! on a dynamic var mutates its THREAD binding; the root is
;; untouched once the binding pops.
;; Oracle (Clojure 1.12, 2026-07-12):
;;   (def ^:dynamic *s* 1)
;;   (binding [*s* 10] (set! *s* (+ *s* 32)) *s*) → 42, then *s* → 1.
(def ^:dynamic *s* 1)
[(binding [*s* 10] (set! *s* (+ *s* 32)) *s*) *s*]
;; expect: [42 1]
