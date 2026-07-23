;; infinite? (fundamentals batch A1): is the number, cast to double (the
;; JVM's ^double param), positive or negative infinity. NaN is not
;; infinite; integers cast cleanly (never infinite); a non-number throws
;; (the JVM's ClassCastException), mirroring NaN? (sorted_builtins.go).
;; oracle (clojure 1.12.5, 2026-07-23): [true true false false false :threw]
[(infinite? ##Inf)
 (infinite? ##-Inf)
 (infinite? 1.5)
 (infinite? 1)
 (infinite? ##NaN)
 (try (infinite? "a") (catch Exception e :threw))]
;; expect: [true true false false false :threw]
