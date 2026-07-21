;; bounded-count (fundamentals audit 2026-07): a counted? coll returns its
;; REAL count regardless of n; an uncounted seq counts at most n elements.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (bounded-count 3 [1 2 3 4 5]) => 5
;;   (bounded-count 10 [1 2 3]) => 3
;;   (bounded-count 2 (map inc (range 100))) => 2
;;   (bounded-count 0 (list 1 2)) => 2
;;   (bounded-count 10 (map inc (range 3))) => 3
[(bounded-count 3 [1 2 3 4 5])
 (bounded-count 10 [1 2 3])
 (bounded-count 2 (map inc (range 100)))
 (bounded-count 0 (list 1 2))
 (bounded-count 10 (map inc (range 3)))]
;; expect: [5 3 2 2 3]
