;; ADR 0040 §2.1: (pipe from to) copies every value from->to and closes to
;; when from closes.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(let [from (async/to-chan! [1 2 3]) to (async/chan 10)]
  (async/pipe from to)
  (async/<!! (async/into [] to)))
;; expect: [1 2 3]
