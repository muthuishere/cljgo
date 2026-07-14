;; set! on a non-assignable expression is an analysis-time error.
;; Oracle (Clojure 1.12, 2026-07-12): "Invalid assignment target".
(set! "notatarget" 3)
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; expect-error: invalid assignment target
