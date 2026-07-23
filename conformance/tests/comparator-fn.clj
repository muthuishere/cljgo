;; Batch A3: comparator — turn a 2-arg predicate into a -1/1/0
;; comparator fn, usable wherever a comparator is (sort).
;; Oracle (clojure 1.12.5): verified 2026-07-23.
[((comparator <) 1 2)
 ((comparator <) 2 1)
 ((comparator <) 1 1)
 (sort (comparator >) [1 3 2])]
;; expect: [-1 1 0 (3 2 1)]
