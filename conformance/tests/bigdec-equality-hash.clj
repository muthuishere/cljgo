;; ADR 0032 / spike S16 oracle finding 1: Clojure `=` on two BigDecimals is
;; equiv/compareTo-based — (= 1.0M 1.00M) is TRUE (Java .equals
;; scale-sensitivity does NOT leak into =), and hasheq normalizes via
;; stripTrailingZeros so the hash-set collapses to one element.
;; Finding 2: cross-category = is false even for equal values; == is true.
;; Oracle: real Clojure 1.12.5 CLI, verified 2026-07-16.
[(= 1.0M 1.00M) (== 1.0M 1.00M)
 (= 1M 1) (== 1M 1) (= 1M 1N) (= 1M 1.0) (== 1M 1.0)
 (= 0.5M 1/2) (== 0.5M 1/2)
 (compare 1.0M 1.00M) (compare 1.0M 1.01M) (compare 2M 1M)
 (< 1.0M 1.01M) (<= 1.0M 1.00M)
 (zero? 0.000M) (pos? 0.000M) (neg? -1.0M)
 (count (hash-set 1.0M 1.00M))]
;; expect: [true true false true false false true false true 0 -1 1 true true true false true 1]
