;; ADR 0040 §2.1: (take n ch) delivers at most n values from ch then closes —
;; fewer when ch has fewer, none when n is 0.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
[(async/<!! (async/into [] (async/take 3 (async/to-chan! [1 2 3 4 5]))))
 (async/<!! (async/into [] (async/take 10 (async/to-chan! [1 2 3]))))
 (async/<!! (async/into [] (async/take 0 (async/to-chan! [1 2]))))]
;; expect: [[1 2 3] [1 2 3] []]
