;; abs (design/08 batch E, ADR 0022): absolute value over the numeric
;; tower (int, float, ratio, NaN), throwing on a non-number.
;; oracle (clojure 1.12.5): [(abs -5) (abs 5) (abs -5.5) (abs 0)
;; (NaN? (abs ##NaN)) (abs ##-Inf) (abs -1/5)] =>
;; [5 5 5.5 0 true ##Inf 1/5]; (abs nil) throws.
[[(abs -5) (abs 5) (abs -5.5) (abs 0) (NaN? (abs ##NaN)) (abs ##-Inf) (abs -1/5)]
 (try (abs nil) :nothrow (catch Exception _e :threw))]
;; expect: [[5 5 5.5 0 true ##Inf 1/5] :threw]
