;; ns-map's mapping set is IDENTICAL between the interpreter and a
;; compiled binary. The println'd count is the canary the dual harness
;; compares byte-for-byte across legs (its value moves as core grows, so
;; it is deliberately NOT the frozen line); the frozen marker is the
;; bootstrap defmacro mapping — interned by corelib.RegisterAll since
;; 2026-07-23, so both boot legs carry it. Before that, only the
;; interpreter (pkg/eval) interned clojure.core/defmacro, and a compiled
;; binary's (ns-map 'user) ran one mapping short — an off-by-one
;; REPL-vs-binary divergence.
;; oracle (clojure 1.12.5):
;;   (contains? (ns-map 'user) 'defmacro)         => true
;;   (contains? (ns-map 'clojure.core) 'defmacro) => true
(println (count (sort (keys (ns-map 'user)))))
[(contains? (ns-map 'user) 'defmacro) (contains? (ns-map 'clojure.core) 'defmacro)]
;; expect: [true true]
