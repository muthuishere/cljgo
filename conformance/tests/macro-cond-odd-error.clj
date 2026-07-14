;; cond with an odd number of forms errors WHILE MACROEXPANDING, as on
;; JVM Clojure. Oracle (clojure 1.12.5): Syntax error macroexpanding
;; cond ... "cond requires an even number of forms".
(cond 1)
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; expect-error: cond requires an even number of forms
