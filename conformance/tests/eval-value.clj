;; eval analyzes + evaluates an already-read form through the REPL path (ADR 0022).
(eval (list '+ 1 2 (eval (list '* 3 4))))
;; expect: 15
