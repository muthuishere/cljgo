;; ADR 0040 §2.2: (reduce f init ch) yields a channel with the single fold
;; result after ch closes; the reduced box short-circuits.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
[(async/<!! (async/reduce + 0 (async/to-chan! [1 2 3 4])))
 (async/<!! (async/reduce (fn [acc x] (if (= x 3) (reduced acc) (+ acc x))) 0 (async/to-chan! [1 2 3 4])))
 (async/<!! (async/reduce + 100 (async/to-chan! [])))]
;; expect: [10 3 100]
