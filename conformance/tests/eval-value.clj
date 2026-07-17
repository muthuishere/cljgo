;; eval analyzes + evaluates an already-read form through the REPL path (ADR 0022).
;; harness: eval — `eval` needs the analyzer, and an AOT-compiled cljgo
;; binary has no compiler linked (ADR 0046 §5, the CLJS model): calling it
;; there throws "eval is not available in an AOT-compiled binary". This is
;; a DEVIATION from the JVM, where AOT code still links clojure.jar and
;; (eval '(+ 1 2)) => 3; it is documented, not silent — the binary says
;; exactly why. Interpreted behavior below is unchanged.
(eval (list '+ 1 2 (eval (list '* 3 4))))
;; expect: 15
