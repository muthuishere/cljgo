;; ADR 0040 §2.3: (untap-all m) removes every tap. Both taps receive the
;; first value, then untap-all clears them so a second source value reaches
;; neither; closing the source lets the (already-untapped) taps be drained.
;; The first value is confirmed via a poll on each after untap-all.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(let [src (async/chan 5) m (async/mult src) t1 (async/chan 5) t2 (async/chan 5) sync (async/chan 5)]
  (async/tap m t1) (async/tap m t2)
  (async/>!! src 1) (async/<!! t1) (async/<!! t2)
  (async/untap-all m)
  (async/tap m sync)
  (async/>!! src 2) (async/<!! sync)
  [(async/poll! t1) (async/poll! t2)])
;; expect: [nil nil]
