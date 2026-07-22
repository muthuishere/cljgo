;; ADR 0040 tier T3 + core-async-audit 2026-07: (pipeline n to xf from)
;; reads from `from`, transforms each value with parallelism n, and writes
;; results to `to` IN INPUT ORDER, closing `to` when `from` drains. Basic
;; parallel map, single-worker (n=1), and an empty source (to closes
;; immediately, so `into` yields []).
;; oracle (JVM core.async 1.6.681, Clojure 1.12.5, fresh 2026-07-22 run):
;;   (pipeline 4 to (map inc) (range 10)) => [1 2 3 4 5 6 7 8 9 10]
;;   (pipeline 1 to (map inc) (range 5))  => [1 2 3 4 5]
;;   (pipeline 2 to (map inc) [])         => []
;; oracle: skip — needs the core.async dep; frozen from the run above
(require '[clojure.core.async :as async])
[(async/<!! (async/into [] (let [to (async/chan 100)]
   (async/pipeline 4 to (map inc) (async/to-chan! (range 10))) to)))
 (async/<!! (async/into [] (let [to (async/chan 100)]
   (async/pipeline 1 to (map inc) (async/to-chan! (range 5))) to)))
 (async/<!! (async/into [] (let [to (async/chan 100)]
   (async/pipeline 2 to (map inc) (async/to-chan! [])) to)))]
;; expect: [[1 2 3 4 5 6 7 8 9 10] [1 2 3 4 5] []]
