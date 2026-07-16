;; BigInt stays a distinct boxed type (clojure.lang.BigInt on the JVM,
;; *lang.BigInt in cljgo), never demoted to a fixed-width long even when the
;; value fits: category is integer? but not int?; arithmetic (+ * quot rem
;; mod, and the '-promoting +' *') preserves or produces bigint-ness exactly
;; as the JVM does; printing keeps the N suffix; equality with longs is
;; category-based (= 1 1N) => true. Backs the suite portability shim's
;; big-int? (core/clojure_test_portability.cljg).
;; oracle (clojure 1.12.5): every row below verified verbatim 2026-07-16.
[[(integer? 1N) (int? 1N) (integer? 1) (int? 1)]
 [(pr-str 1N) (pr-str (+ 1N 1)) (pr-str (+' 9223372036854775807 1)) (pr-str (*' -9223372036854775808 -1))]
 [(int? (+ 1 1)) (int? (+' 1 1)) (int? (* 2 3)) (int? (*' 2 3))]
 [(int? (rem 5N 2)) (integer? (rem 5N 2)) (int? (mod 5N 2)) (integer? (mod 5N 2)) (int? (quot 5N 2)) (integer? (quot 5N 2))]
 [(int? (rem 3 1/2)) (integer? (rem 3 1/2)) (int? (quot 3 1/2)) (integer? (quot 3 1/2))]
 [(= 1 1N) (= 1N 1) (== 1 1N) (= 2N (+ 1N 1)) (= 2 (+ 1N 1))]]
;; expect: [[true false true true] ["1N" "2N" "9223372036854775808N" "9223372036854775808N"] [true true true true] [false true false true false true] [false true false true] [true true true true true]]
