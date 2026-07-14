;; fn parameter destructuring (clojure.core/fn via maybe-destructured): each
;; param vector is destructured, so [[a b] {:keys [c]}] binds positionally and
;; associatively from the two arguments.
;; oracle: ((fn [[a b] {:keys [c]}] [a b c]) [1 2] {:c 3}) => [1 2 3]
;;   (JVM Clojure 1.12.5, clojure CLI)
((fn [[a b] {:keys [c]}] [a b c]) [1 2] {:c 3})
;; expect: [1 2 3]
