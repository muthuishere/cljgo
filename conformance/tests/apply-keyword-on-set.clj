;; Applying a keyword to a set (as apply's spread arg) looks the key up like
;; `get`, returning the key itself when present — same as `(:a #{:a :b :c})`.
;; A keyword's IFn.Invoke previously only special-cased Associative, so a
;; set (not Associative in this codebase) fell through to the default value.
;; oracle (clojure 1.12.5): (apply :a [#{:a :b :c}]) => :a
(apply :a [#{:a :b :c}])
;; expect: :a
