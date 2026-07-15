;; var? is true only for Vars (ADR 0022).
(list (var? (var map)) (var? map) (var? 5))
;; expect: (true false false)
