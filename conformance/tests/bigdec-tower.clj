;; ADR 0032 / spike S16: tower interaction. Longs/BigInts/Ratios promote INTO
;; the decimal category ((+ 1 1.0M) is 2.0M); doubles win OVER it
;; ((+ 1.0 1.0M) is the double 2.0 — which is also why a BigDecimal can meet
;; ##NaN/##Inf without ever holding them). Conversions out: int/long truncate
;; toward zero, bigint yields N, biginteger prints bare, rationalize is exact.
;; Oracle: real Clojure 1.12.5 CLI, verified 2026-07-16.
[(+ 1 1.0M) (+ 1N 1.0M) (+ 1/2 0.5M) (+ 1.0 1.0M)
 (+ 1.0M ##NaN) (+ 1.0M ##Inf)
 (* 2 1.50M)
 (int 1.9M) (long 1.9M) (double 1.5M)
 (bigint 1.5M) (biginteger 1.5M)
 (rationalize 1.10M) (number? 1.0M)]
;; expect: [2.0M 2.0M 1.0M 2.0 ##NaN ##Inf 3.00M 1 1 1.5 1N 1 11/10 true]
