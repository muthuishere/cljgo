;; `int?` (fixed-precision integer) and `integer?` (fixed-precision OR
;; arbitrary-precision BigInt) are different predicates — 1N is integer? but
;; not int?. Both previously shared lang.IsInteger, which accepts *BigInt,
;; so (int? 1N) wrongly returned true (and neg-int?/pos-int?, which are
;; defined in terms of int?, followed suit).
;; oracle (clojure 1.12.5): [(int? 1N) (integer? 1N) (int? 1) (integer? 1)]
;; => [false true true true]
[(int? 1N) (integer? 1N) (int? 1) (integer? 1)]
;; expect: [false true true true]
