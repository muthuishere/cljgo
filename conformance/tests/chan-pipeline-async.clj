;; ADR 0040 tier T3: (pipeline-async n to af from) transforms each input
;; with an ASYNC fn af = (fn [val result-ch]) that delivers 0+ results to
;; result-ch and CLOSES it. Multiple emits per input are flushed in order;
;; an input whose af emits nothing contributes nothing; results stay in
;; input order across parallelism. With close?=false `to` is left open.
;; oracle (JVM core.async 1.6.681, Clojure 1.12.5, fresh 2026-07-22 run):
;;   af emits (*100) and (+1 *100)  over [1 2 3]      => [100 101 200 201 300 301]
;;   af emits (*10) only for odd x  over [1 2 3 4]     => [10 30]
;;   close?=false, af emits (*10)   over [1 2 3]       => [10 20 30 nil] (to open)
;; oracle: skip — needs the core.async dep; frozen from the run above
(require '[clojure.core.async :as async])
[(async/<!! (async/into [] (let [to (async/chan 100)]
   (async/pipeline-async 3 to
     (fn [x res] (async/go (async/>! res (* x 100)) (async/>! res (+ 1 (* x 100))) (async/close! res)))
     (async/to-chan! [1 2 3])) to)))
 (async/<!! (async/into [] (let [to (async/chan 100)]
   (async/pipeline-async 2 to
     (fn [x res] (async/go (when (odd? x) (async/>! res (* x 10))) (async/close! res)))
     (async/to-chan! [1 2 3 4])) to)))
 (let [to (async/chan 100)]
   (async/pipeline-async 2 to
     (fn [x res] (async/go (async/>! res (* x 10)) (async/close! res)))
     (async/to-chan! [1 2 3]) false)
   (async/<!! (async/timeout 300))
   [(async/<!! to) (async/<!! to) (async/<!! to) (async/poll! to)])]
;; expect: [[100 101 200 201 300 301] [10 30] [10 20 30 nil]]
