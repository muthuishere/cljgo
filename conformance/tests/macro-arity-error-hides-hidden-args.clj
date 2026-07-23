;; A macro invoked with the wrong number of args reports the USER-VISIBLE
;; count: the macro fn actually receives &form and &env prepended, and
;; Compiler.macroexpand1 hides them by rethrowing
;; ArityException(e.actual - 2, e.name). Verified vs clojure 1.12.5:
;;   (defmacro mm [a b] 1) (macroexpand '(mm 1))
;;   => Wrong number of args (1) passed to: user/mm   (not 3)
(defmacro mm [a b] 1)
(mm 1)
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; expect-error: wrong number of args (1) passed to: user/mm
