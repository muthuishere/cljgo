;; split-with (fundamentals audit 2026-07): predicate-based sibling of
;; split-at — [(take-while pred coll) (drop-while pred coll)].
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (split-with #(< % 3) [1 2 3 4 1]) => [(1 2) (3 4 1)]
;;   (split-with pos? []) => [() ()]
;;   (split-with odd? [2 4 6]) => [() (2 4 6)]
;;   (split-with odd? [1 3 5]) => [(1 3 5) ()]
[(split-with #(< % 3) [1 2 3 4 1])
 (split-with pos? [])
 (split-with odd? [2 4 6])
 (split-with odd? [1 3 5])]
;; expect: [[(1 2) (3 4 1)] [() ()] [() (2 4 6)] [(1 3 5) ()]]
