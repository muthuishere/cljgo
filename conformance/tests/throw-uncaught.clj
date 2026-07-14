;; An uncaught throw propagates. Oracle (clojure 1.12.5):
;; (throw (ex-info "nope" {})) => Execution error (ExceptionInfo) ... "nope".
(throw (ex-info "nope" {}))
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; expect-error: nope
