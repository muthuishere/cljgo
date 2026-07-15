;; Transient vector: conj! mutates in place and must be re-bound; persistent!
;; freezes back to an immutable vector (design/08 §5 Batch 3, ADR 0022).
;; oracle: (persistent! (conj! (conj! (conj! (transient []) 1) 2) 3)) => [1 2 3]
;;   (JVM Clojure 1.12.5, clojure CLI)
(persistent! (conj! (conj! (conj! (transient []) 1) 2) 3))
;; expect: [1 2 3]
