;; defmulti/defmethod: dispatch on a value the dispatch fn produces (here
;; the :shape keyword), with a :default fallback method.
;; Verified vs Clojure CLI 1.12.5 (/tmp oracle):
;;   (defmulti area :shape)
;;   (defmethod area :circle [s] (* 3.14 (:r s) (:r s)))
;;   (defmethod area :square [s] (* (:side s) (:side s)))
;;   (defmethod area :default [s] :unknown)
;;   [(area {:shape :circle :r 10}) (area {:shape :square :side 3}) (area {:shape :tri})]
;;   => [314.0 9 :unknown]
;; expect: [314.0 9 :unknown]
(defmulti area :shape)
(defmethod area :circle [s] (* 3.14 (:r s) (:r s)))
(defmethod area :square [s] (* (:side s) (:side s)))
(defmethod area :default [s] :unknown)

[(area {:shape :circle :r 10})
 (area {:shape :square :side 3})
 (area {:shape :tri})]
