;; areduce (fundamentals batch 1): folds an array — idx counts up, ret
;; rebinds to expr from init, returns ret; both idx and ret are visible
;; in expr.
;; oracle (clojure 1.12.5): over (int-array [1 2 3]):
;; (areduce ar i acc 0 (+ acc (aget ar i))) => 6;
;; (areduce ar i acc 100 (+ acc i)) => 103.
(def ar (int-array [1 2 3]))
[(areduce ar i acc 0 (+ acc (aget ar i)))
 (areduce ar i acc 100 (+ acc i))]
;; expect: [6 103]
