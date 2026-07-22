;; ADR 0040 §2.1: (to-chan! coll) is a fresh channel of coll's values that
;; closes when they are exhausted; (into [] ch) drains it into a vector.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(async/<!! (async/into [] (async/to-chan! [1 2 3])))
;; expect: [1 2 3]
