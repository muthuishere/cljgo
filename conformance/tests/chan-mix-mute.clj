;; ADR 0040 §2.5: a muted input is consumed but not forwarded. toggle adds c2
;; already muted (the atomic add-in-a-state that makes the change race-free),
;; so only c1's values reach out; c2's value is dropped.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(let [out (async/chan 10) mx (async/mix out) c1 (async/chan 10) c2 (async/chan 10)]
  (async/admix mx c1) (async/toggle mx {c2 {:mute true}})
  (async/>!! c1 1) (async/>!! c2 2) (async/>!! c1 3)
  [(async/<!! out) (async/<!! out) (async/poll! out)])
;; expect: [1 3 nil]
