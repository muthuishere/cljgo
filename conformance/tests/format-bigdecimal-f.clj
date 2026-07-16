;; ADR 0030/0032 follow-on (S14 VERDICT.md deferred tail, S16 item 14):
;; format %f on a BigDecimal renders Java's Formatter exactly at the
;; value's OWN scale (BigDecimal.setScale(precision, HALF_UP) then plain
;; notation) -- no double round-trip, so huge/tiny magnitudes never
;; switch to E-notation the way %f on a double effectively can't either,
;; but the DIGITS are exact where a double would have binary noise.
;; Oracle: real Clojure 1.12.5 CLI, verified 2026-07-16.
[(format "%.2f" 1.005M)
 (format "%f" 123M)
 (format "%.0f" 1.5M)
 (format "%,.2f" 12345.678M)
 (format "%.20f" 1.1M)
 (format "%.2f" -1.005M)
 (format "%.2f" -0M)
 (format "%.5f" 1E-10M)
 (format "%.10f" 100M)
 (format "%(.2f" -5.5M)
 (format "%+.2f" 5.5M)
 (format "%,.2f" -12345.678M)]
;; expect: ["1.01" "123.000000" "2" "12,345.68" "1.10000000000000000000" "-1.01" "0.00" "0.00000" "100.0000000000" "(5.50)" "+5.50" "-12,345.68"]
