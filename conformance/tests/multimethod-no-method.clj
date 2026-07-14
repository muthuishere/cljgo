;; A multimethod with no matching method and no :default throws.
;; Verified vs Clojure CLI 1.12.5 (/tmp oracle):
;;   (defmulti h identity)
;;   (defmethod h 1 [x] :one)
;;   (h 2) => IllegalArgumentException
;;          "No method in multimethod 'h' for dispatch value: 2"
(defmulti h identity)
(defmethod h 1 [x] :one)
(h 2)
;; harness: eval — expects an error; v0 has no compiled error-output contract
;; expect-error: No method in multimethod 'h' for dispatch value: 2
