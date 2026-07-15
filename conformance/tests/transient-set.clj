;; Transient set: conj! adds, disj! removes (variadic), persistent! freezes
;; (design/08 §5 Batch 3, ADR 0022). Compared by = so the result is
;; independent of hash-set print order.
;; oracle: (= #{3 4} (persistent! (disj! (conj! (transient #{1 2 3}) 4) 1 2))) => true
;;   (JVM Clojure 1.12.5, clojure CLI)
(= #{3 4} (persistent! (disj! (conj! (transient #{1 2 3}) 4) 1 2)))
;; expect: true
