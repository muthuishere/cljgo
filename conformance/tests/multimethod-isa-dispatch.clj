;; Multimethod isa?-based dispatch (fundamentals audit 2026-07, the
;; prefer-method batch): a dispatch value derived from a method's key
;; resolves to that method through the global hierarchy — including
;; vector dispatch values — and an exact method still beats a parent's.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   after (derive ::rect ::shape) and (defmethod area ::shape ...):
;;   (area ::rect) => :shape; (get-method area ::rect) is the ::shape fn;
;;   (defmethod g [::shape ::shape] ...) matches (g ::rect ::rect);
;;   after adding (defmethod area ::rect ...): (area ::rect) => :rect
(derive ::rect ::shape)
(defmulti area (fn [x] x))
(defmethod area ::shape [_] :shape)
(defmulti g (fn [a b] [a b]))
(defmethod g [::shape ::shape] [_ _] :two-shapes)
(def first-lookup (area ::rect))
(def had-method (some? (get-method area ::rect)))
(def no-method (get-method area ::nope))
(defmethod area ::rect [_] :rect)
[first-lookup had-method no-method (g ::rect ::rect) (area ::rect)]
;; expect: [:shape true nil :two-shapes :rect]
