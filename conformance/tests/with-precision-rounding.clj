;; ADR 0032 follow-on (S16 items 13-14, spikes/s16-bigdecimal-scaled/
;; probes_wp.clj rows wp1): with-precision's :rounding modes, at
;; precision 1 (Java's MathContext(1, MODE)), on (* 1.1M 1M) and its
;; siblings — every RoundingMode the clojure-test-suite's
;; with_precision.cljc exercises (clojuredocs examples).
;; Oracle: real Clojure 1.12.5 CLI, verified 2026-07-16.
[(with-precision 1 :rounding UP (* 1.1M 1M))
 (with-precision 1 :rounding CEILING (* 1.1M 1M))
 (with-precision 1 :rounding UP (* -1.1M 1M))
 (with-precision 1 :rounding CEILING (* -1.1M 1M))
 (with-precision 1 :rounding DOWN (* 1.9M 1M))
 (with-precision 1 :rounding FLOOR (* 1.9M 1M))
 (with-precision 1 :rounding DOWN (* -1.9M 1M))
 (with-precision 1 :rounding FLOOR (* -1.9M 1M))
 (with-precision 1 :rounding HALF_EVEN (* 1.5M 1M))
 (with-precision 1 :rounding HALF_EVEN (* 2.5M 1M))
 (with-precision 1 :rounding HALF_EVEN (* -1.5M 1M))
 (with-precision 1 :rounding HALF_EVEN (* -2.5M 1M))
 (with-precision 1 :rounding HALF_UP (* 1.5M 1M))
 (with-precision 1 :rounding HALF_DOWN (* 1.5M 1M))
 (with-precision 1 :rounding HALF_UP (* -1.5M 1M))
 (with-precision 1 :rounding HALF_DOWN (* -1.5M 1M))
 (with-precision 1 :rounding UNNECESSARY (* 2M 1M))]
;; expect: [2M 2M -2M -1M 1M 1M -1M -2M 2M 2M -2M -2M 2M 1M -2M -1M 2M]
