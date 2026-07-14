;; max/min/max-key/min-key/mod/quot/rem and numeric predicates (clojure.core).
;; oracle (clojure 1.12.5):
;;   (max 1 5 3) => 5 ; (min 4 2 8) => 2
;;   (max-key count "a" "ccc" "bb") => "ccc" ; (min-key count "a" "ccc" "bb") => "a"
;;   (mod 7 3) => 1 ; (quot 7 3) => 2 ; (rem 7 3) => 1 ; (mod -7 3) => 2
;;   (even? 4) (odd? 3) (zero? 0) (pos? 2) (neg? -1) => true...
;;   (<= 1 2 2) => true ; (>= 3 3 1) => true
[(max 1 5 3)
 (min 4 2 8)
 (max-key count "a" "ccc" "bb")
 (min-key count "a" "ccc" "bb")
 (mod 7 3)
 (quot 7 3)
 (rem 7 3)
 (mod -7 3)
 (even? 4)
 (odd? 3)
 (zero? 0)
 (pos? 2)
 (neg? -1)
 (<= 1 2 2)
 (>= 3 3 1)]
;; expect: [5 2 "ccc" "a" 1 2 1 2 true true true true true true true]
