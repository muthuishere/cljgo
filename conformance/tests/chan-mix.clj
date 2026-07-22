;; ADR 0040 §2.5: (mix out) merges admixed inputs into out. Two inputs feed
;; four values; order across inputs is nondeterministic, so the drained take
;; is sorted before freezing.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(let [out (async/chan 10) mx (async/mix out) c1 (async/chan 10) c2 (async/chan 10)]
  (async/admix mx c1) (async/admix mx c2)
  (async/>!! c1 1) (async/>!! c2 2) (async/>!! c1 3) (async/>!! c2 4)
  (sort (async/<!! (async/into [] (async/take 4 out)))))
;; expect: (1 2 3 4)
