;; cond with an odd number of forms errors WHILE MACROEXPANDING, as on
;; JVM Clojure. Oracle (clojure 1.12.5): Syntax error macroexpanding
;; cond ... "cond requires an even number of forms".
(cond 1)
;; expect-error: cond requires an even number of forms
