(defprotocol Greet (greet [this]))
;; simulate reify by hand: a map of method-name->closure capturing a local
(defn make-anon [prefix]
  {:greet (fn [self] (str prefix "world"))})
(let [obj (make-anon "Hi, ")]
  ;; manual dispatch: pick the impl off the value, call with the value as this
  (println ((:greet obj) obj)))
