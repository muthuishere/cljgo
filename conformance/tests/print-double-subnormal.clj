;; Subnormal doubles follow the JDK's exact (Ryū-based) rendering, which
;; is not always the strictly shortest round-trip form: Double/MIN_VALUE
;; is "4.9E-324" (not "5E-324"), and 1.0E-322 reads to the double that
;; prints "9.9E-323". A subnormal only shortens to one significant digit
;; when its 2-digit rounding ends in 0 (2.0E-323).
;; Expectation frozen from real Clojure 1.12.5 (clojure CLI, JDK 26), 2026-07-12:
;;   (pr-str [4.9E-324 9.9E-324 2.0E-323 1.0E-322])
;;   => "[4.9E-324 9.9E-324 2.0E-323 9.9E-323]"
[4.9E-324 9.9E-324 2.0E-323 1.0E-322]
;; expect: [4.9E-324 9.9E-324 2.0E-323 9.9E-323]
