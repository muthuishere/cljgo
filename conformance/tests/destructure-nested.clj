;; Nested destructuring: a vector pattern whose slots are themselves a vector
;; pattern and a map pattern. Recurses through the same -pb dispatch.
;; oracle: (let [[[a b] {:keys [c]}] [[1 2] {:c 3}]] [a b c]) => [1 2 3]
;;   (JVM Clojure 1.12.5, clojure CLI)
(let [[[a b] {:keys [c]}] [[1 2] {:c 3}]]
  [a b c])
;; expect: [1 2 3]
