;; ADR 0040 §2.1: (onto-chan! ch coll) pumps coll onto ch and closes ch
;; (default close? true), so a later (into [] ch) drains and terminates.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(let [c (async/chan 10)]
  (async/<!! (async/onto-chan! c [10 20 30]))
  (async/<!! (async/into [] c)))
;; expect: [10 20 30]
