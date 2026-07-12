;; Non-finite doubles pr as ##Inf / ##-Inf / ##NaN (Clojure's
;; print-method for Double), both as literals and as computed values.
;; (str ##Inf) is "Infinity" — that Java name is only for str, not pr.
;; Expectation frozen from real Clojure 1.12.5 (clojure CLI, JDK 26), 2026-07-12:
;;   (pr-str [##Inf ##-Inf ##NaN (/ 1.0 0.0) (- (/ 1.0 0.0))])
;;   => "[##Inf ##-Inf ##NaN ##Inf ##-Inf]"
[##Inf ##-Inf ##NaN (/ 1.0 0.0) (- (/ 1.0 0.0))]
;; expect: [##Inf ##-Inf ##NaN ##Inf ##-Inf]
