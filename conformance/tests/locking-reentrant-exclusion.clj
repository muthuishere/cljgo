;; locking (fundamentals batch 1): body runs holding a per-object
;; monitor and its value is returned; reentrant (nested locking on the
;; same object from the same thread runs); empty body => nil; mutual
;; exclusion makes 50 concurrent non-atomic vswap! increments under the
;; lock deterministic.
;; oracle (clojure 1.12.5): (locking o (+ 1 2)) => 3;
;; (locking o (locking o :reentrant)) => :reentrant; (locking o) => nil;
;; the 50-future volatile increment under locking => 50.
(def o (atom 0))
(def vv (volatile! 0))
(def futs (doall (map (fn [_] (future (locking vv (vswap! vv inc))))
                      (range 50))))
(doseq [f futs] @f)
[(locking o (+ 1 2))
 (locking o (locking o :reentrant))
 (locking o)
 @vv]
;; expect: [3 :reentrant nil 50]
