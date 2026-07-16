;; ADR 0032 follow-on (S16 items 13-14, probes_wp.clj rows wp2-wp5):
;; default rounding mode (HALF_UP) division under *math-context*, and
;; +/-/* consulting *math-context* the same way real Clojure's
;; BigDecimalOps does (Numbers.java) — including a negative-scale result
;; ("wp2 big": (with-precision 2 (+ 123M 0M)) => 1.2E+2M).
;; Oracle: real Clojure 1.12.5 CLI, verified 2026-07-16.
[(with-precision 2 (/ 1M 3M))
 (with-precision 5 (/ 1M 3M))
 (with-precision 2 (/ 2M 3M))
 (with-precision 3 (+ 1.2345M 0M))
 (with-precision 3 (- 1.2345M 0M))
 (with-precision 3 (* 1.2345M 1M))
 (with-precision 4 :rounding HALF_DOWN (/ 1M 3M))
 (with-precision 2 (+ 123M 0M))]
;; expect: [0.33M 0.33333M 0.67M 1.23M 1.23M 1.23M 0.3333M 1.2E+2M]
