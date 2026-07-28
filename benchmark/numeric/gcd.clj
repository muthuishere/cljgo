;; zero? as the loop-termination test + rem in the recur value: the
;; classic Euclid shape, and the most common idiom the batch unlocks.
(defn gcd [a b]
  (if (zero? b) a (recur b (rem a b))))

(loop [i 1 s 0]
  (if (< i 600000)
    (recur (inc i) (+ s (gcd i 123456)))
    (println s)))
