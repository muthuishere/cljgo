;; clojure.core/mix-collection-hash and hash-combine (ADR 0051), the two
;; integer-in / integer-out primitives the collection hashes are built
;; from. mix-collection-hash is Murmur3.mixCollHash(hash-basis, count);
;; hash-combine is clojure.lang.Util.hashCombine over two ints (boost's
;; seed ^ (h + 0x9e3779b9 + (seed<<6) + (seed>>2))). Matches JVM 1.12.5.
[(mix-collection-hash 100 2)
 (mix-collection-hash 0 0)
 (mix-collection-hash 736442005 3)
 (hash-combine 5 7)
 (hash-combine 0 0)
 (hash-combine 100 200)]
;; expect: [-308053282 -15128758 -1661653191 -1640531196 -1640531527 -1640524802]
