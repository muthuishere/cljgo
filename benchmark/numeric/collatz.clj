;; even? / quot / max — the predicate is the loop's branch test and `max`
;; is the outer carrier, so before the batch neither loop typed at all.
(defn collatz-len [n]
  (loop [x n c 0]
    (if (= x 1)
      c
      (recur (if (even? x) (quot x 2) (inc (* 3 x))) (inc c)))))

(loop [i 1 best 0]
  (if (< i 150000)
    (recur (inc i) (max best (collatz-len i)))
    (println best)))
