;; ADR 0040 §2.1: (merge chans) fans in a collection of channels into one,
;; closing when all inputs close. Interleave order is nondeterministic, so
;; the result is sorted before freezing.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(sort (async/<!! (async/into [] (async/merge [(async/to-chan! [1 2]) (async/to-chan! [3 4])]))))
;; expect: (1 2 3 4)
