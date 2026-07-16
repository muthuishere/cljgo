;; ADR 0032 / spike S16: decimal division is exact-or-throw. `/` returns
;; the exact quotient at preferred scale sx−sy (padded when shorter), throws
;; on a non-terminating expansion (denominator must reduce to 2^a·5^b) and
;; on a zero divisor — never Inf. A long divisor promotes to a scale-0
;; BigDecimal first, so (/ 1M 3) throws exactly like (/ 1M 3M).
;; quot follows divideToIntegralValue's preferred-scale rules —
;; (quot 10.0M 3) is 3.0M but (quot 10.0M 3.0M) is 3M.
;; Oracle: real Clojure 1.12.5 CLI, verified 2026-07-16.
[(/ 1M 4M) (/ 10.0M 2M) (/ 1M 2) (/ 1.0M 8)
 (try (/ 1M 3M) :nothrow (catch Exception _e :threw))
 (try (/ 1M 3) :nothrow (catch Exception _e :threw))
 (try (/ 1M 0M) :nothrow (catch Exception _e :threw))
 (quot 10.0M 3) (quot 10M 3) (quot 10.0M 3.0M)
 (rem 10.0M 3) (rem 10.0M 3.0M) (rem -10.0M 3)
 (mod 10.0M 3) (mod 10.0M 3.0M) (mod -10.0M 3)]
;; expect: [0.25M 5.0M 0.5M 0.125M :threw :threw :threw 3.0M 3M 3M 1.0M 1.0M -1.0M 1.0M 1.0M 2.0M]
