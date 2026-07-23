;; pmap's variadic arity: f applied across parallel colls, stopping at
;; the shortest — same shape as map, computed via parallel futures.
;; oracle (Clojure 1.12.5 CLI, 2026-07-23): (pmap + [1 2 3] [10 20 30])
;; => (11 22 33)
(pmap + [1 2 3] [10 20 30])
;; expect: (11 22 33)
