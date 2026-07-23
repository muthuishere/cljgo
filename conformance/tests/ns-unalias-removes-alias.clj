;; ns-unalias removes an alias from the namespace; removing an alias that
;; never existed is a nil-returning no-op.
;; oracle (Clojure 1.12.5 CLI, 2026-07-23):
;; (create-ns 'scratch.aa) (alias 'sa 'scratch.aa) (ns-unalias *ns* 'sa)
;; then [(contains? (ns-aliases *ns*) 'sa) (ns-unalias *ns* 'never-here)]
;; => [false nil]
(create-ns 'scratch.aa)
(alias 'sa 'scratch.aa)
(ns-unalias *ns* 'sa)
[(contains? (ns-aliases *ns*) 'sa) (ns-unalias *ns* 'never-here)]
;; expect: [false nil]
