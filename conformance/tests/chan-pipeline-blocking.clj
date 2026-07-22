;; ADR 0040 tier T3 (#9): pipeline-blocking behaves IDENTICALLY to pipeline
;; here. On the JVM the two differ only by executor (a bounded compute pool
;; vs the unbounded blocking pool); on the Go host every worker is a
;; goroutine, so the distinction collapses and both call the same engine —
;; documented observable equality, not an invented difference. Same map and
;; mapcat behaviour as chan-pipeline* under the -blocking name.
;; oracle (JVM core.async 1.6.681, Clojure 1.12.5, fresh 2026-07-22 run):
;;   (pipeline-blocking 3 (map inc) (range 5))        => [1 2 3 4 5]
;;   (pipeline-blocking 2 (mapcat #(vector % %)) [1 2]) => [1 1 2 2]
;; oracle: skip — needs the core.async dep; frozen from the run above
(require '[clojure.core.async :as async])
[(async/<!! (async/into [] (let [to (async/chan 100)]
   (async/pipeline-blocking 3 to (map inc) (async/to-chan! (range 5))) to)))
 (async/<!! (async/into [] (let [to (async/chan 100)]
   (async/pipeline-blocking 2 to (mapcat (fn [x] [x x])) (async/to-chan! [1 2])) to)))]
;; expect: [[1 2 3 4 5] [1 1 2 2]]
