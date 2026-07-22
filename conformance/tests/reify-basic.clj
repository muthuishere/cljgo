;; reify: an anonymous value satisfying a protocol, its method bodies closing
;; over an enclosing local, with multi-arity methods and `this`. satisfies?
;; is true for the declared protocol, false for a value that does not.
;; Verified vs Clojure CLI 1.12.5 (scratch oracle):
;;   (defprotocol Greet (greet [this] [this x]) (name-of [this]))
;;   (let [prefix "Hi, "] (reify Greet (greet [this] (str prefix "world")) ...))
;;   => ["Hi, world" "Hi, Bob" "anon" true false]
;; expect: ["Hi, world" "Hi, Bob" "anon" true false]
(defprotocol Greet
  (greet [this] [this x])
  (name-of [this]))

(let [prefix "Hi, "]
  (let [r (reify Greet
            (greet [this] (str prefix "world"))
            (greet [this x] (str prefix x))
            (name-of [this] "anon"))]
    [(greet r) (greet r "Bob") (name-of r) (satisfies? Greet r) (satisfies? Greet "x")]))
