;; find-var resolves a fully-qualified symbol to its Var (ADR 0022).
(var? (find-var 'clojure.core/reduce))
;; expect: true
