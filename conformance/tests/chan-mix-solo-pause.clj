;; ADR 0040 §2.5: with (solo-mode m :pause), non-solo inputs are PAUSED (not
;; consumed) while a solo is active. c1 solo forwards its 1; c2 non-solo is
;; paused, so it retains its 2.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(let [out (async/chan 10) mx (async/mix out) c1 (async/chan 10) c2 (async/chan 10)]
  (async/solo-mode mx :pause) (async/toggle mx {c1 {:solo true} c2 {}})
  (async/>!! c1 1) (async/>!! c2 2)
  [(async/<!! out) (async/<!! c2)])
;; expect: [1 2]
