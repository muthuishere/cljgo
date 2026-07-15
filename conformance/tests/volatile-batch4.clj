;; Batch 4 volatile! (ADR 0022, ADR 0024). *lang.Volatile is a bare mutable
;; box (Deref/Reset), deliberately not compare-and-set — vswap!/vreset!
;; are plain read-compute-write, matching the JVM's clojure.lang.Volatile.
;; Oracle (clojure 1.12.5): (volatile? (volatile! 1)) => true;
;; (volatile? (atom 1)) => false; vswap! applies f to the deref'd value
;; plus args, stores and returns it; vreset! unconditionally replaces.
[(let [v (volatile! 1)]
   [(volatile? v) @v
    (vswap! v inc) @v
    (vswap! v + 10) @v
    (vreset! v 100) @v
    (volatile? (atom 1)) (volatile? 1)])]
;; expect: [[true 1 2 2 12 12 100 100 false false]]
