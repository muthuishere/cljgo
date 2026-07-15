;; The transient substrate for fast into/reduce: reduce conj! over a transient
;; vector, then persistent!, must round-trip to the eager vector of the same
;; seq (design/08 §5 Batch 3).
;; oracle: (= (vec (range 100)) (persistent! (reduce conj! (transient []) (range 100)))) => true
;;   (JVM Clojure 1.12.5, clojure CLI)
(= (vec (range 100))
   (persistent! (reduce conj! (transient []) (range 100))))
;; expect: true
