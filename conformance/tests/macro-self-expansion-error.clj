;; A self-expanding macro hits the analyzer's expansion limit — a
;; positioned error, not a stack overflow. (DEVIATION: JVM Clojure has
;; no limit and overflows the stack.) The expansion is built fresh each
;; time; a macro returning its own input form IDENTICALLY stops the
;; loop (identity check), matching Compiler.macroexpand1's ret != x.
(defmacro m [] (list 'm))
(m)
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; expect-error: too many macroexpansions
