;; ADR 0040 §2.5: when any input is soloed, non-solo inputs are muted under
;; the default solo-mode :mute. Both channels are added atomically (c1 solo,
;; c2 normal), so only c1's values reach out.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(let [out (async/chan 10) mx (async/mix out) c1 (async/chan 10) c2 (async/chan 10)]
  (async/toggle mx {c1 {:solo true} c2 {}})
  (async/>!! c1 1) (async/>!! c2 2) (async/>!! c1 3)
  [(async/<!! out) (async/<!! out) (async/poll! out)])
;; expect: [1 3 nil]
