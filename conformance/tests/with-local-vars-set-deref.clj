;; with-local-vars: names bound to fresh un-interned dynamic Vars,
;; thread-bound to their inits for the body's extent — set via var-set,
;; read via deref; the names ARE var objects (var? => true).
;; oracle (Clojure 1.12.5 CLI, 2026-07-23):
;; (with-local-vars [x 1 y 2] (var-set x (+ @x @y)) [@x @y]) => [3 2];
;; (with-local-vars [x 1] (var? x)) => true
[(with-local-vars [x 1 y 2] (var-set x (+ @x @y)) [@x @y])
 (with-local-vars [x 1] (var? x))]
;; expect: [[3 2] true]
