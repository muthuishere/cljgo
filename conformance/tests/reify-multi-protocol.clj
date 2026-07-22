;; reify: one anonymous value satisfying TWO protocols, built inside a fn so
;; its methods close over the fn's parameters. satisfies? is true for both.
;; Verified vs Clojure CLI 1.12.5 (scratch oracle):
;;   (defprotocol Area (area [this])) (defprotocol Named (nm [this]))
;;   (defn make-shape [w h label] (reify Area (area [this] (* w h))
;;                                       Named (nm [this] label)))
;;   (let [s (make-shape 3 4 "rect")] [(area s) (nm s) (satisfies? Area s)
;;                                      (satisfies? Named s)])
;;   => [12 "rect" true true]
;; expect: [12 "rect" true true]
(defprotocol Area (area [this]))
(defprotocol Named (nm [this]))

(defn make-shape [w h label]
  (reify
    Area (area [this] (* w h))
    Named (nm [this] label)))

(let [s (make-shape 3 4 "rect")]
  [(area s) (nm s) (satisfies? Area s) (satisfies? Named s)])
