;; ADR 0040 §2.3: (untap m ch) removes one tap; the remaining tap keeps
;; receiving. Both taps see the first value (a barrier confirming the mult is
;; live), then t2 is untapped so only t1 receives the second value.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(let [src (async/chan 5) m (async/mult src) t1 (async/chan 5) t2 (async/chan 5)]
  (async/tap m t1) (async/tap m t2)
  (async/>!! src 1) (async/<!! t1) (async/<!! t2)
  (async/untap m t2)
  (async/>!! src 2) (async/close! src)
  (async/<!! (async/into [] t1)))
;; expect: [2]
