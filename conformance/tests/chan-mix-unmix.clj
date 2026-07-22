;; ADR 0040 §2.5: (unmix m ch) removes an input — its values are no longer
;; consumed or forwarded. c1 barrier values flush the state change through the
;; mix pump; afterwards c2's value is neither forwarded (poll out is nil) nor
;; consumed (c2 retains its 2).
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(let [out (async/chan 10) mx (async/mix out) c1 (async/chan 10) c2 (async/chan 10)]
  (async/admix mx c1) (async/admix mx c2) (async/unmix mx c2)
  (async/>!! c1 :b1) (async/<!! out) (async/>!! c1 :b2) (async/<!! out)
  (async/>!! c2 2)
  (async/>!! c1 :b3) (async/<!! out)
  [(async/poll! out) (async/<!! c2)])
;; expect: [nil 2]
