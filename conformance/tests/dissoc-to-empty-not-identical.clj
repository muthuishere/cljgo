;; dissoc'ing a map's last key must NOT return the shared canonical EMPTY
;; singleton — only the literal `{}` / no-arg `(hash-map)` do. Map.Without
;; previously routed through NewMap, which special-cases a zero-length
;; key/val slice into the cached emptyMap, making two independently-emptied
;; maps wrongly `identical?`.
;; oracle (clojure 1.12.5): (identical? (hash-map) (dissoc (hash-map :a 1) :a))
;; => false ; (identical? (hash-map) (hash-map)) => true
[(identical? (hash-map) (dissoc (hash-map :a 1) :a))
 (identical? (hash-map) (hash-map))]
;; expect: [false true]
