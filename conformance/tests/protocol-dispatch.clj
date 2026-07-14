;; defprotocol + deftype: methods dispatch on the type of the first arg.
;; Verified vs Clojure CLI 1.12.5 (/tmp oracle):
;;   (defprotocol Greet (greet [this]) (tag [this n]))
;;   (deftype Person [nm] Greet ...) => ["hi sam" "sam:42"]
;; expect: ["hi sam" "sam:42"]
(defprotocol Greet
  (greet [this])
  (tag [this n]))

(deftype Person [nm]
  Greet
  (greet [this] (str "hi " nm))
  (tag [this n] (str nm ":" n)))

(def p (->Person "sam"))
[(greet p) (tag p 42)]
