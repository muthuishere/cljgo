;; float64 quot/rem/mod guard ##Inf/##NaN operands and a 0.0 divisor like
;; JVM Numbers.quotient/remainder(double,double) (ADR 0029 cluster A, spike
;; S13): non-finite quotients throw "Infinite or NaN", a 0.0 divisor throws
;; "Divide by zero", and the in-range/finite-huge edges keep JVM values —
;; (quot 1 ##Inf) is 0.0, (rem 1 ##-Inf) is ##NaN (0*Inf = NaN), and a
;; finite quotient beyond int64 comes back as a double, not a BigDecimal.
;; oracle (clojure 1.12.5): expectation vector below, byte-identical.
[(try (quot ##Inf 1) (catch Throwable e (ex-message e)))
 (try (quot 1 ##NaN) (catch Throwable e (ex-message e)))
 (try (rem ##-Inf 1) (catch Throwable e (ex-message e)))
 (try (rem 10.0 0) (catch Throwable e (ex-message e)))
 (try (mod 10.0 0) (catch Throwable e (ex-message e)))
 (try (mod 5 ##NaN) (catch Throwable e (ex-message e)))
 (quot 1 ##Inf)
 (rem 1 ##Inf)
 (rem 1 ##-Inf)
 (mod 1 ##Inf)
 (quot 1e300 1.0)
 (rem 1e300 3.0)
 (quot 10.0 3.0)
 (rem -10.0 3)]
;; expect: ["Infinite or NaN" "Infinite or NaN" "Infinite or NaN" "Divide by zero" "Divide by zero" "Infinite or NaN" 0.0 ##NaN ##NaN ##NaN 1.0E300 0.0 3.0 -1.0]
