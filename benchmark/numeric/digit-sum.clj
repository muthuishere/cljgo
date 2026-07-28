;; quot + rem in the recur carriers — before the second-table batch a
;; single untyped `quot` demoted BOTH carriers, so the whole fn stayed boxed.
(defn digit-sum [n]
  (loop [x n s 0]
    (if (< x 1) s (recur (quot x 10) (+ s (rem x 10))))))

(loop [i 0 acc 0]
  (if (< i 400000)
    (recur (inc i) (+ acc (digit-sum i)))
    (println acc)))
