;; clojure.core/hash over collections (ADR 0051): vectors and lists mix
;; their element hasheqs order-sensitively (Murmur3.hashOrdered), maps and
;; sets order-independently (Murmur3.hashUnordered). A list and vector of
;; the same elements hash equal; nesting composes. Matches JVM 1.12.5.
;; Oracle: clojure 1.12.5 (hash x) for each value.
[(hash [1 2]) (hash [1 2 3]) (hash (list 1 2 3))
 (hash #{1 2 3}) (hash {:a 1 :b 2})
 (hash []) (hash [:a :b]) (hash {}) (hash [[1] [2]])]
;; expect: [156247261 736442005 736442005 439094965 161871944 -2017569654 -781497396 -15128758 536877500]
