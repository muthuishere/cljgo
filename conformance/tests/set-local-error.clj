;; set! on a local is an analysis-time error (locals are immutable).
;; Oracle (Clojure 1.12, 2026-07-12): "Cannot assign to non-mutable: q".
(let* [q 1] (set! q 2))
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; expect-error: assign to non-mutable: q
