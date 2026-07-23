;; compare-and-set! / swap-vals! / reset-vals! / reset-meta! (fundamentals
;; batch A1). compare-and-set! compares the CURRENT value to the expected
;; one — identity-shaped, so an equal-but-fresh collection does NOT match
;; (the JVM's boxed-object identity; cljgo's Go interface ==, which is
;; pointer identity for collections and value equality for scalars — a
;; documented superset for uncached scalar boxes, essentials_builtins.go).
;; swap-vals!/reset-vals! return the [old new] pair as a vector;
;; reset-meta! wholesale-replaces an iref's metadata and returns it.
;; oracle (clojure 1.12.5, 2026-07-23): the exact vector below;
;; (class (swap-vals! (atom 1) inc)) => clojure.lang.PersistentVector.
[(let [a (atom 1)] [(compare-and-set! a 1 2) @a (compare-and-set! a 99 3) @a])
 (let [a (atom [1 2])] (compare-and-set! a [1 2] :x))
 (let [a (atom 1)] [(swap-vals! a inc) (swap-vals! a + 10)])
 (vector? (swap-vals! (atom 1) inc))
 (let [a (atom 1)] [(reset-vals! a 5) @a])
 (let [a (atom 1 :meta {:a 1})] [(reset-meta! a {:b 2}) (meta a)])]
;; expect: [[true 2 false 2] false [[1 2] [2 12]] true [[1 5] 5] [{:b 2} {:b 2}]]
