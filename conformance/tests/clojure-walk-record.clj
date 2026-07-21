;; clojure.walk over a defrecord (core/walk.cljg): the record branch rebuilds
;; by conj-ing walked entries back onto the record, so the result stays a
;; record of the same type.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (defrecord Pt [x y])
;;   (postwalk #(if (number? %) (inc %) %) (->Pt 1 2)) => #user.Pt{:x 2, :y 3}
;;   (record? that) => true
(require '[clojure.walk :as w])
(defrecord Pt [x y])
(def walked (w/postwalk (fn [f] (if (number? f) (inc f) f)) (->Pt 1 2)))
[walked (record? walked)]
;; expect: [#user.Pt{:x 2, :y 3} true]
