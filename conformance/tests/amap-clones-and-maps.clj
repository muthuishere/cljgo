;; amap (fundamentals batch 1): binds ret to (aclone a), asets expr at
;; each idx, returns ret — the source array is untouched, and expr sees
;; the clone through ret.
;; oracle (clojure 1.12.5): over (int-array [1 2 3]):
;; (amap ar i ret (* 2 (aget ar i))) => [2 4 6], source stays [1 2 3];
;; (amap ar i ret (+ (aget ret i) 10)) => [11 12 13].
(def ar (int-array [1 2 3]))
(def doubled (amap ar i ret (* 2 (aget ar i))))
[(vec doubled) (vec ar) (vec (amap ar i ret (+ (aget ret i) 10)))]
;; expect: [[2 4 6] [1 2 3] [11 12 13]]
