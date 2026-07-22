;; ADR 0040 tier T3: pipeline PRESERVES INPUT ORDER even with parallelism
;; n>1 and a transform whose per-value latency is non-monotonic. Each
;; transform parks on a `timeout` whose length depends on (mod x 5), so
;; earlier inputs often finish LAST — yet the output is strictly in input
;; order [0 1 … 7]. This is THE pipeline contract (the mechanism: the
;; dispatcher enqueues one result channel per input in order, and the
;; writer drains them in that same order). pipeline-blocking (here scaling
;; by 10) preserves order identically — on Go both are the same
;; goroutine-parallel engine (ADR 0040 #9).
;; oracle (JVM core.async 1.6.681, Clojure 1.12.5, fresh 2026-07-22 run):
;;   pipeline          => [0 1 2 3 4 5 6 7]
;;   pipeline-blocking => [0 10 20 30 40 50 60 70]
;; oracle: skip — needs the core.async dep; frozen from the run above
(require '[clojure.core.async :as async])
[(async/<!! (async/into []
   (let [to (async/chan 100)]
     (async/pipeline 4 to
       (map (fn [x] (async/<!! (async/timeout (* 15 (- 5 (mod x 5))))) x))
       (async/to-chan! (range 8)))
     to)))
 (async/<!! (async/into []
   (let [to (async/chan 100)]
     (async/pipeline-blocking 4 to
       (map (fn [x] (async/<!! (async/timeout (* 15 (- 5 (mod x 5))))) (* x 10)))
       (async/to-chan! (range 8)))
     to)))]
;; expect: [[0 1 2 3 4 5 6 7] [0 10 20 30 40 50 60 70]]
