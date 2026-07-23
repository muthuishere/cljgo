;; class vs type (fundamentals batch A1): class is nil for nil and
;; otherwise the value's host type; type is class PLUS the :type-metadata
;; override — the one JVM difference, frozen the same way here. cljgo's
;; class value is the reflect.Type stand-in `type` already uses
;; (comparable via =, misc_builtins.go), so the frozen shape is the
;; =-comparisons, not a printed class name.
;; oracle (clojure 1.12.5, 2026-07-23): [nil true true :T true true]
[(class nil)
 (= (class 1) (type 1))
 (= (class "s") (type "s"))
 (type (with-meta {} {:type :T}))
 (= (class (with-meta {} {:type :T})) (class {}))
 (= (class 1.5) (type 1.5))]
;; expect: [nil true true :T true true]
