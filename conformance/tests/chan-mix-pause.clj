;; ADR 0040 §2.5: a paused input is NOT consumed — its value stays in the
;; channel. toggle adds c2 already paused, so c1's 1 reaches out while c2
;; retains its 2.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(let [out (async/chan 10) mx (async/mix out) c1 (async/chan 10) c2 (async/chan 10)]
  (async/admix mx c1) (async/toggle mx {c2 {:pause true}})
  (async/>!! c1 1) (async/>!! c2 2)
  [(async/<!! out) (async/<!! c2)])
;; expect: [1 2]
