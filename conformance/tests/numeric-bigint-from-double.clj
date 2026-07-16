;; bigint/biginteger of a double convert exactly the way the JVM does
;; (ADR 0029 cluster B, spike S13): through BigDecimal.valueOf(double) —
;; the shortest round-trip DECIMAL representation truncated toward zero —
;; never a saturating int64 cast. So Double/MAX_VALUE becomes the full
;; 309-digit integer, 4.611686018427388E18 becomes its decimal reading
;; 4611686018427388000N (NOT the exact binary value ...7904N), and
;; ##Inf/##NaN throw "Infinite or NaN" (NumberFormatException on the JVM).
;; oracle (clojure 1.12.5): expectation vector below, byte-identical.
[(bigint 1.7976931348623157e+308)
 (bigint 4.611686018427388e18)
 (bigint -1.5)
 (bigint 1.5)
 (= (bigint 1.7976931348623157e+308) (biginteger 1.7976931348623157e+308))
 (try (bigint ##Inf) (catch Throwable e (ex-message e)))
 (try (bigint ##NaN) (catch Throwable e (ex-message e)))]
;; expect: [179769313486231570000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000N 4611686018427388000N -1N 1N true "Infinite or NaN" "Infinite or NaN"]
