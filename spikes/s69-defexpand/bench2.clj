;; s69 bench2 — a CALL-OVERHEAD-DOMINATED body (no allocation), which is
;; where inlining is supposed to pay. N = 3,000,000 iterations of (* x x).
(def nano (deref (var clojure.core/-nano-time)))
(def N 3000000)

(defn sqf [x] (* x x))
(defmacro sqm [x] (list '* x x))
(defexpand sqe [x] (* x x))

(defn ms [start] (/ (double (- (nano) start)) 1000000.0))

(defn b-hand [] (let [t (nano)] (loop [i 0 acc 0] (if (< i N) (recur (inc i) (+ acc (* i i))) [(ms t) acc]))))
(defn b-fn   [] (let [t (nano)] (loop [i 0 acc 0] (if (< i N) (recur (inc i) (+ acc (sqf i))) [(ms t) acc]))))
(defn b-mac  [] (let [t (nano)] (loop [i 0 acc 0] (if (< i N) (recur (inc i) (+ acc (sqm i))) [(ms t) acc]))))
(defn b-exp  [] (let [t (nano)] (loop [i 0 acc 0] (if (< i N) (recur (inc i) (+ acc (sqe i))) [(ms t) acc]))))

(defn best [f n]
  (reduce (fn [x y] (if (< x y) x y)) (map (fn [_] (first (f))) (range n))))

(defn -main []
  (b-hand) (b-fn) (b-mac) (b-exp)
  (let [h (best b-hand 5) w (best b-fn 5) m (best b-mac 5) e (best b-exp 5)]
    (println "hand     " h "ms  ratio 1.000")
    (println "defn     " w "ms  ratio" (/ w h))
    (println "defmacro " m "ms  ratio" (/ m h))
    (println "defexpand" e "ms  ratio" (/ e h))))

(-main)
