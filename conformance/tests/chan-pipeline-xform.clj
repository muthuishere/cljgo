;; ADR 0040 tier T3: the pipeline transducer may EXPAND (mapcat: one input
;; -> many outputs), FILTER (fewer outputs), or be stateful. Crucially the
;; xf is applied PER INPUT on a fresh (chan 1 xf) transducer channel, so a
;; stateful/aggregating transducer does NOT accumulate across inputs:
;; (partition-by odd?) over [1 3 2 4] yields a singleton partition per
;; input ([[1] [3] [2] [4]]), NOT the [[1 3] [2 4]] a single seq would give.
;; oracle (JVM core.async 1.6.681, Clojure 1.12.5, fresh 2026-07-22 run):
;;   (pipeline 3 (mapcat #(vector % %)) [1 2 3]) => [1 1 2 2 3 3]
;;   (pipeline 3 (filter even?) (range 10))      => [0 2 4 6 8]
;;   (pipeline 2 (partition-by odd?) [1 3 2 4])  => [[1] [3] [2] [4]]
;; oracle: skip — needs the core.async dep; frozen from the run above
(require '[clojure.core.async :as async])
[(async/<!! (async/into [] (let [to (async/chan 100)]
   (async/pipeline 3 to (mapcat (fn [x] [x x])) (async/to-chan! [1 2 3])) to)))
 (async/<!! (async/into [] (let [to (async/chan 100)]
   (async/pipeline 3 to (filter even?) (async/to-chan! (range 10))) to)))
 (async/<!! (async/into [] (let [to (async/chan 100)]
   (async/pipeline 2 to (partition-by odd?) (async/to-chan! [1 3 2 4])) to)))]
;; expect: [[1 1 2 2 3 3] [0 2 4 6 8] [[1] [3] [2] [4]]]
