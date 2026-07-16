;; abs over the whole numeric tower (ADR 0029 cluster E, spike S13 — the
;; per-type Ops.Abs were already vendored; this pins the clojure.core
;; wiring). Edges: (abs Long/MIN_VALUE) is Long/MIN_VALUE (JVM Math.abs
;; 2's-complement identity), (abs -0.0) is 0.0, (abs ##-Inf) is ##Inf,
;; (abs ##NaN) is NaN, and nil throws.
;; oracle (clojure 1.12.5): expectation vector below, byte-identical.
[(abs -1)
 (abs 1)
 (abs -1.0)
 (abs -0.0)
 (abs ##-Inf)
 (abs -123.456M)
 (abs -123N)
 (abs -9223372036854775808)
 (abs -1/5)
 (NaN? (abs ##NaN))
 (try (abs nil) true (catch Throwable e :threw))]
;; expect: [1 1 1.0 0.0 ##Inf 123.456M 123N -9223372036854775808 1/5 true :threw]
