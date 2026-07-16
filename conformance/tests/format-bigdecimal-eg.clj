;; ADR 0030/0032 follow-on (S14 VERDICT.md deferred tail, S16 item 14):
;; format %e/%E/%g on a BigDecimal -- forced/chosen scientific notation
;; computed exactly from the value's own unscaled digits (no double
;; round-trip). %g picks fixed vs. scientific by the post-rounding
;; adjusted exponent against [-4, precision), same algorithm as %g on a
;; double (formatDirectGFloat) but decimal-exact.
;; Oracle: real Clojure 1.12.5 CLI, verified 2026-07-16.
[(format "%.2e" 12345.6789M)
 (format "%e" 12345.6789M)
 (format "%E" 12345.6789M)
 (format "%e" 0M)
 (format "%.0e" 9.999M)
 (format "%e" 0.0000001234M)
 (format "%.2e" 0M)
 (format "%.2e" 1E+200M)
 (format "%.2e" -12345.6789M)
 (format "%(.2e" -12345.6789M)
 (format "%.3g" 12345.6789M)
 (format "%.3g" 123456M)
 (format "%.3g" 12.3456M)
 (format "%g" 0.0001234M)
 (format "%g" 123456789M)
 (format "%.2g" 0M)]
;; expect: ["1.23e+04" "1.234568e+04" "1.234568E+04" "0.000000e+00" "1e+01" "1.234000e-07" "0.00e+00" "1.00e+200" "-1.23e+04" "(1.23e+04)" "1.23e+04" "1.23e+05" "12.3" "0.000123400" "1.23457e+08" "0.0"]
