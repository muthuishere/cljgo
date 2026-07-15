;; Single-threaded ownership: once persistent! freezes a transient, any further
;; op on the old handle throws (design/08 §5 Batch 3, ADR 0022).
(let [t (transient {})]
  (persistent! t)
  (assoc! t :x 1))
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; expect-error: transient used after persistent! call
