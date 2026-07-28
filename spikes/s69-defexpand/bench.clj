;; s69 bench — hand-written vs defn wrapper vs defexpand vs defmacro.
;; N = 300_000 iterations of (swap! a conj x) on a vector atom.
;; Same file runs interpreted (cljgo run) and compiled (cljgo build).

(def nano (deref (var clojure.core/-nano-time)))
(def N 300000)

(defn wrap! [a x] (swap! a conj x))
(defmacro macro-add! [a x] (list 'swap! a 'conj x))
(defexpand add! [a x] (swap! a conj x))

(defn ms [start] (/ (double (- (nano) start)) 1000000.0))

(defn bench-hand []
  (let [a (atom []) t (nano)]
    (dotimes [i N] (swap! a conj i))
    [(ms t) (count @a)]))

(defn bench-wrap []
  (let [a (atom []) t (nano)]
    (dotimes [i N] (wrap! a i))
    [(ms t) (count @a)]))

(defn bench-macro []
  (let [a (atom []) t (nano)]
    (dotimes [i N] (macro-add! a i))
    [(ms t) (count @a)]))

(defn bench-expand []
  (let [a (atom []) t (nano)]
    (dotimes [i N] (add! a i))
    [(ms t) (count @a)]))

(defn best [f n]
  (let [rs (map (fn [_] (first (f))) (range n))]
    (reduce (fn [x y] (if (< x y) x y)) rs)))

(defn -main []
  ;; warm
  (bench-hand) (bench-wrap) (bench-macro) (bench-expand)
  (let [h (best bench-hand 5)
        w (best bench-wrap 5)
        m (best bench-macro 5)
        e (best bench-expand 5)]
    (println "hand    " h "ms  ratio 1.000")
    (println "defn    " w "ms  ratio" (/ w h))
    (println "defmacro" m "ms  ratio" (/ m h))
    (println "defexpand" e "ms  ratio" (/ e h))))

(-main)
