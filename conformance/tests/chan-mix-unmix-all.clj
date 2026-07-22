;; ADR 0040 §2.5: (unmix-all m) removes every input. A fresh c3 admixed after
;; unmix-all carries a barrier value to out — proving the pump has processed
;; the state past unmix-all — so c1/c2's later values are neither forwarded
;; (poll out nil) nor consumed (c1/c2 retain their values).
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(let [out (async/chan 10) mx (async/mix out) c1 (async/chan 10) c2 (async/chan 10) c3 (async/chan 10)]
  (async/admix mx c1) (async/admix mx c2) (async/unmix-all mx)
  (async/admix mx c3) (async/>!! c3 :barrier) (async/<!! out)
  (async/>!! c1 1) (async/>!! c2 2)
  [(async/poll! out) (async/<!! c1) (async/<!! c2)])
;; expect: [nil 1 2]
