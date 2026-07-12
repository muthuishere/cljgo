;; recur rebinds simultaneously: all args evaluate BEFORE any rebinding,
;; so (recur b a) swaps.
;; Oracle (Clojure 1.12, 2026-07-12): (loop* [i 0 a 1 b 2] (if (< i 1) (recur (+ i 1) b a) [a b])) → [2 1].
(loop* [i 0 a 1 b 2]
  (if (< i 1)
    (recur (+ i 1) b a)
    [a b]))
;; expect: [2 1]
