;; defmacro attr-map parity: (defmacro name doc-string? attr-map? ...) with
;; an optional TRAILING attr-map, same prefix grammar as defn. :arglists
;; records the user-visible params — the hidden &form/&env are not part of
;; them (clojure.core/defmacro does the same).
;;
;; oracle (clojure 1.12.5, `clojure -M -e "<the forms below>"`):
;; [7 1 "mdoc" ([x]) true 8 2 ([x])]
(defmacro m1 "mdoc" {:mm 1} [x] x)
(defmacro m2 ([x] x) {:tt 2})
(defmacro m3 [x] x)
[(m1 7) (:mm (meta #'m1)) (:doc (meta #'m1)) (:arglists (meta #'m1))
 (boolean (:macro (meta #'m1)))
 (m2 8) (:tt (meta #'m2)) (:arglists (meta #'m3))]
;; expect: [7 1 "mdoc" ([x]) true 8 2 ([x])]
