;; ADR 0040 §2.3: (mult ch) fans every source value out to every tapped
;; channel; closing the source closes each tap (close? default true), so both
;; taps drain the same [1 2 3] and terminate.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(let [src (async/chan 5) m (async/mult src) t1 (async/chan 5) t2 (async/chan 5)]
  (async/tap m t1) (async/tap m t2)
  (async/onto-chan! src [1 2 3])
  [(async/<!! (async/into [] t1)) (async/<!! (async/into [] t2))])
;; expect: [[1 2 3] [1 2 3]]
