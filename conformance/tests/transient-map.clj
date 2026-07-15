;; Transient map: assoc!/dissoc!/conj! mutate a mutable map handle, persistent!
;; freezes it (design/08 §5 Batch 3, ADR 0022). conj! takes a [k v] pair.
;; oracle: (persistent! (dissoc! (conj! (assoc! (assoc! (transient {}) :a 1) :b 2) [:c 3]) :b)) => {:a 1, :c 3}
;;   (JVM Clojure 1.12.5, clojure CLI)
(persistent!
 (dissoc! (conj! (assoc! (assoc! (transient {}) :a 1) :b 2) [:c 3]) :b))
;; expect: {:a 1, :c 3}
