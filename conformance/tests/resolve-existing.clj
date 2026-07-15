;; resolve returns the Var a symbol names in the current namespace (ADR 0022).
(var? (resolve 'map))
;; expect: true
