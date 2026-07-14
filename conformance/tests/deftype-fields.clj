;; deftype field access: fields are bare-symbol locals inside method bodies
;; and are readable externally via (.-f x). Verified vs Clojure CLI 1.12.5:
;;   (deftype Pair [a b] P (sum [this] (+ a b))) => [3 4 7]
;; expect: [3 4 7]
(defprotocol P (sum [this]))

(deftype Pair [a b]
  P
  (sum [this] (+ a b)))

(def x (->Pair 3 4))
[(.-a x) (.-b x) (sum x)]
