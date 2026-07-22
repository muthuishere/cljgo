;; ADR 0040 §2.1: (split p ch t-buf f-buf) routes truthy-pred values to the
;; first channel, the rest to the second; both close when ch closes. Buffers
;; let each side be drained sequentially without deadlock.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(let [[t f] (async/split even? (async/to-chan! [1 2 3 4 5 6]) 10 10)]
  [(async/<!! (async/into [] t)) (async/<!! (async/into [] f))])
;; expect: [[2 4 6] [1 3 5]]
