;; defmulti with a computed dispatch fn (an anonymous fn, not just a
;; keyword): dispatch value is the fn's result.
;; Verified vs Clojure CLI 1.12.5 (/tmp oracle):
;;   (defmulti g (fn [x] (if (even? x) :even :odd)))
;;   (defmethod g :even [x] (str x " is even"))
;;   (defmethod g :odd  [x] (str x " is odd"))
;;   [(g 4) (g 7)] => ["4 is even" "7 is odd"]
;; expect: ["4 is even" "7 is odd"]
(defmulti g (fn [x] (if (even? x) :even :odd)))
(defmethod g :even [x] (str x " is even"))
(defmethod g :odd [x] (str x " is odd"))

[(g 4) (g 7)]
