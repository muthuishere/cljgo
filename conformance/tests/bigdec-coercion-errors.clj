;; ADR 0032 / spike S16: BigDecimal can never hold Inf/NaN — coercing a
;; non-finite double throws (JVM: "Infinite or NaN"), as do nil, malformed
;; strings, and non-numeric types. Wording is host-specific (same scoring
;; policy as S13/S14); only the THREW is asserted.
;; Oracle: real Clojure 1.12.5 CLI, verified 2026-07-16 — all rows throw.
[(try (bigdec ##Inf) :nothrow (catch Exception _e :threw))
 (try (bigdec ##-Inf) :nothrow (catch Exception _e :threw))
 (try (bigdec ##NaN) :nothrow (catch Exception _e :threw))
 (try (bigdec nil) :nothrow (catch Exception _e :threw))
 (try (bigdec "abc") :nothrow (catch Exception _e :threw))
 (try (bigdec "") :nothrow (catch Exception _e :threw))
 (try (bigdec true) :nothrow (catch Exception _e :threw))
 (try (bigdec :a) :nothrow (catch Exception _e :threw))]
;; expect: [:threw :threw :threw :threw :threw :threw :threw :threw]
