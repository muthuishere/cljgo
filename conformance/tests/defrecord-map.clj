;; defrecord is map-backed: get/:key/assoc/keys/count/= (by value AND type)
;; all work, and a record is never = to a plain map (either direction).
;; Verified vs Clojure CLI 1.12.5:
;;   (defrecord Rect [w h] Shape ...) =>
;;   [2 3 6 "rect" true false false 2 (:w :h)]
;; expect: [2 3 6 "rect" true false false 2 (:w :h)]
(defprotocol Shape
  (area [this])
  (nm [this]))

(defrecord Rect [w h]
  Shape
  (area [this] (* w h))
  (nm [this] "rect"))

(def r (->Rect 2 3))
[(:w r) (get r :h) (area r) (nm r)
 (= r (->Rect 2 3)) (= r (->Rect 2 9))
 (= r {:w 2 :h 3}) (count r) (keys r)]
