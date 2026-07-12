;; and/or value semantics. Oracle (clojure 1.12.5):
;; [(and) (and 1 2) (and false 2) (and nil) (or) (or nil false 3) (or 1 2)]
;; => [true 2 false nil nil 3 1]
[(and) (and 1 2) (and false 2) (and nil) (or) (or nil false 3) (or 1 2)]
;; expect: [true 2 false nil nil 3 1]
