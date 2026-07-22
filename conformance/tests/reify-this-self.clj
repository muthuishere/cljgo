;; reify: `this` is the value itself — a method can call another method on it,
;; while both close over an enclosing local.
;; Verified vs Clojure CLI 1.12.5 (scratch oracle):
;;   (defprotocol Counter (val-of [this]) (twice [this]))
;;   (let [n 21] (reify Counter (val-of [this] n)
;;                              (twice [this] (* 2 (val-of this)))))
;;   => [21 42]
;; expect: [21 42]
(defprotocol Counter
  (val-of [this])
  (twice [this]))

(let [n 21]
  (let [c (reify Counter
            (val-of [this] n)
            (twice [this] (* 2 (val-of this))))]
    [(val-of c) (twice c)]))
