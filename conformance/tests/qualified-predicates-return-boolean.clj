;; qualified-keyword?/qualified-symbol?/qualified-ident? must return a real
;; boolean, not whatever falsy value the `and` chain happened to short-
;; circuit on. `(and (keyword? x) (namespace x) true)` returns nil (not
;; false) when x has no namespace, because `and` yields its last evaluated
;; (falsy) operand rather than coercing to boolean; real Clojure wraps the
;; whole chain in `boolean`, which core/predicates.cljg was missing.
;; oracle (clojure 1.12.5):
;; [(qualified-keyword? :a) (qualified-keyword? :a/b)
;;  (qualified-symbol? 'a) (qualified-symbol? 'a/b)
;;  (qualified-ident? :a) (qualified-ident? :a/b)]
;; => [false true false true false true]
[(qualified-keyword? :a) (qualified-keyword? :a/b)
 (qualified-symbol? 'a) (qualified-symbol? 'a/b)
 (qualified-ident? :a) (qualified-ident? :a/b)]
;; expect: [false true false true false true]
