;; NaN must poison min/max regardless of position, even against ±Infinity.
;; Go's math.Min/math.Max special-case ±Inf ahead of NaN (math.Min(-Inf, NaN)
;; == -Inf, not NaN), which is the opposite of IEEE-754/Clojure's rule that
;; any NaN operand makes the result NaN. lang.Min/Max previously delegated
;; straight to math.Min/math.Max for the float64/float64 case without an
;; explicit NaN guard.
;; oracle (clojure 1.12.5): (min ##-Inf ##NaN ##Inf) => ##NaN ;
;; (max ##-Inf ##NaN ##Inf) => ##NaN
[(min ##-Inf ##NaN ##Inf) (max ##-Inf ##NaN ##Inf)]
;; expect: [##NaN ##NaN]
