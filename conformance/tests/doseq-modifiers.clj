;; doseq :let / :when / :while modifiers, order-sensitive and scoped to the
;; nearest preceding seq binding: :when skips the body but keeps iterating,
;; :while stops the governing binding's loop, :let binds locals (suite
;; doseq.cljc).
;; oracle (clojure 1.12.5): [[1 4] [0 1 2] [1 4 9]]
(let [a (atom []) b (atom []) c (atom [])]
  (doseq [x (range 4) :let [y (* x x)] :when (pos? y) :while (< y 5)]
    (swap! a conj y))
  (doseq [x (range) :while (< x 3)]
    (swap! b conj x))
  (doseq [x (range 4) :let [y (* x x)] y [y] :when (pos? y)]
    (swap! c conj y))
  [@a @b @c])
;; expect: [[1 4] [0 1 2] [1 4 9]]
