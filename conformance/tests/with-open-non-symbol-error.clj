;; with-open (fundamentals batch 1): a non-symbol binding form is a
;; macroexpansion-time error.
;; oracle (clojure 1.12.5): (with-open [[a] x] a) => Syntax error
;; macroexpanding with-open ... "with-open only allows Symbols in
;; bindings" (IllegalArgumentException).
;; harness: eval — expects an expansion error; cljgo build fails at compile/eval time; v0 has no compiled error-output contract
(with-open [[a] (chan 1)] a)
;; expect-error: with-open only allows Symbols in bindings
