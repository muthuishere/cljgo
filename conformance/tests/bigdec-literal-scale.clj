;; ADR 0032 / spike S16: BigDecimal literals preserve scale and E-notation
;; exactly (unscaled big.Int + int32 scale, Java's model) and print via the
;; javadoc toString plain-vs-E algorithm. The long literal is the old
;; big.Float representation's silent-corruption case — it must stay exact.
;; Oracle: real Clojure 1.12.5 CLI, verified 2026-07-16.
[1M 1.0M 1.00M 1.10M -1.10M 100M 0.000M -0.0M 123.456M
 1E+2M 1e2M 1.23E3M 12345E-2M 1E10M 1E-10M
 0.000001M 0.0000001M
 123456789012345678901234567890.12M]
;; expect: [1M 1.0M 1.00M 1.10M -1.10M 100M 0.000M 0.0M 123.456M 1E+2M 1E+2M 1.23E+3M 123.45M 1E+10M 1E-10M 0.000001M 1E-7M 123456789012345678901234567890.12M]
