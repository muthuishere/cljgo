;; Ratios: / yields a reduced ratio, numerator/denominator (BigIntegers,
;; no N suffix), rationalize of an exact decimal, and == across categories.
[(/ 6 4)
 (numerator 6/4)
 (denominator 6/4)
 (+ 1/2 1/3)
 (* 2/3 3/4)
 (rationalize 0.1)
 (rationalize 1.5)
 (== 1/2 0.5)
 (= 1/2 0.5)]
;; expect: [3/2 3 2 5/6 1/2 1/10 3/2 true false]
