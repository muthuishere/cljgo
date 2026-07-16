;; (int x) range-checks against JAVA's 32-bit int, never the platform int
;; (ADR 0029 cluster F, spike S13). Oracle 1.12.5 shapes: an integral value
;; outside int32 throws ArithmeticException "integer overflow" (longs,
;; BigInts, BigDecimals, Ratios in long range — Math.toIntExact); a double
;; outside int32 throws "Value out of range for int: <Double.toString>";
;; a BigInt beyond long range throws "Value out of range for long: <n>"
;; (RT.longCast fails first). In-range values truncate toward zero.
;; oracle (clojure 1.12.5): expectation vector below, byte-identical.
[(int 2147483647)
 (int -2147483648)
 (int -1.1)
 (int 3/2)
 (try (int 2147483648) (catch Throwable e (ex-message e)))
 (try (int -2147483649) (catch Throwable e (ex-message e)))
 (try (int (bigint 3000000000)) (catch Throwable e (ex-message e)))
 (try (int 2147483647.000001) (catch Throwable e (ex-message e)))
 (try (int -2147483648.000001) (catch Throwable e (ex-message e)))
 (try (int (bigint 1e20)) (catch Throwable e (ex-message e)))]
;; expect: [2147483647 -2147483648 -1 1 "integer overflow" "integer overflow" "integer overflow" "Value out of range for int: 2.147483647000001E9" "Value out of range for int: -2.147483648000001E9" "Value out of range for long: 100000000000000000000"]
