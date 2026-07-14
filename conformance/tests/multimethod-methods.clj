;; methods / get-method / remove-method operate on a multimethod's flat
;; dispatch table.
;; Verified vs Clojure CLI 1.12.5 (/tmp oracle):
;;   (defmulti area :shape)
;;   (defmethod area :circle [s] :c)
;;   (defmethod area :square [s] :s)
;;   [(count (methods area))
;;    (nil? (get-method area :nope))
;;    (nil? (get-method area :circle))
;;    (do (remove-method area :circle) (count (methods area)))]
;;   => [2 true false 1]
;; expect: [2 true false 1]
(defmulti area :shape)
(defmethod area :circle [s] :c)
(defmethod area :square [s] :s)

[(count (methods area))
 (nil? (get-method area :nope))
 (nil? (get-method area :circle))
 (do (remove-method area :circle) (count (methods area)))]
