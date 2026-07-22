;; ADR 0040 §2.2: (into coll ch) conj's every value of ch onto coll (matching
;; clojure.core into's per-collection semantics) and delivers it once ch
;; closes — vector appends, list prepends, set dedups.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
[(async/<!! (async/into [] (async/to-chan! [1 2 3])))
 (async/<!! (async/into () (async/to-chan! [1 2 3])))
 (sort (async/<!! (async/into #{} (async/to-chan! [1 2 2 3]))))]
;; expect: [[1 2 3] (3 2 1) (1 2 3)]
