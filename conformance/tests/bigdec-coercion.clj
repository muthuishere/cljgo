;; ADR 0032 / spike S16: bigdec coercion from every tower type + strings.
;; Ints/BigInts become scale-0 unscaled values directly (exact beyond 2^53);
;; doubles follow BigDecimal.valueOf(double) — shortest decimal string, so
;; (bigdec -0.0) is 0.0M and (bigdec 1.5e300) is 1.5E+300M, not a 301-digit
;; plain integer; ratios divide exactly.
;; Oracle: real Clojure 1.12.5 CLI, verified 2026-07-16.
[(bigdec 1) (bigdec 0) (bigdec -1) (bigdec 1N)
 (bigdec 123456789012345678901234567890N)
 (bigdec 1.0) (bigdec 0.0) (bigdec -1.0) (bigdec -0.0) (bigdec 0.1)
 (bigdec 1.5e300) (bigdec 1.0M) (bigdec 1/2) (bigdec -1/2)
 (bigdec "0") (bigdec "1") (bigdec "+1") (bigdec "-1")
 (bigdec "0.5") (bigdec "-0.5") (bigdec "1.10")
 (bigdec "1e10") (bigdec "1E10") (bigdec "+1e10") (bigdec "-1e10")
 (bigdec "1e+10") (bigdec "1e-10") (bigdec "+1e-10") (bigdec "-1e-10")
 (bigdec "1.23e2") (bigdec "123e2") (bigdec ".5")
 (decimal? (bigdec 1)) (decimal? 1.0)]
;; expect: [1M 0M -1M 1M 123456789012345678901234567890M 1.0M 0.0M -1.0M 0.0M 0.1M 1.5E+300M 1.0M 0.5M -0.5M 0M 1M 1M -1M 0.5M -0.5M 1.10M 1E+10M 1E+10M 1E+10M -1E+10M 1E+10M 1E-10M 1E-10M -1E-10M 123M 1.23E+4M 0.5M true false]
