;; clojure.core/hash-ordered-coll (ADR 0051): the order-sensitive Murmur3
;; mix over a sequence's element hasheqs (seed 1, then mix-collection-hash
;; with the count) — the exact value vectors and lists hash to. A list and
;; vector of the same elements agree. Matches JVM Clojure 1.12.5.
[(hash-ordered-coll [1 2 3])
 (hash-ordered-coll (list 1 2 3))
 (hash-ordered-coll [])
 (hash-ordered-coll [:a :b])]
;; expect: [736442005 736442005 -2017569654 -781497396]
