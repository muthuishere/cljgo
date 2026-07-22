;; clojure.core/hash-unordered-coll (ADR 0051): the order-independent
;; Murmur3 mix over a collection's element hasheqs (sum, then
;; mix-collection-hash with the count) — the value sets and maps hash to.
;; For a map the elements are map entries. Matches JVM Clojure 1.12.5.
[(hash-unordered-coll #{1 2 3})
 (hash-unordered-coll {:a 1 :b 2})
 (hash-unordered-coll [])]
;; expect: [439094965 161871944 -15128758]
