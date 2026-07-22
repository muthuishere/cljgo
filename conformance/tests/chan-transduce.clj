;; ADR 0040 §2.2: (transduce xform f init ch) folds ch through a transducer,
;; running the completion arity at the end. (map inc) over [1 2 3] summed = 9.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(async/<!! (async/transduce (map inc) + 0 (async/to-chan! [1 2 3])))
;; expect: 9
