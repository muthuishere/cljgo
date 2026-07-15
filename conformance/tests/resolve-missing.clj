;; resolve returns nil for an unresolvable symbol — never throws, so the
;; portability shim's when-var-exists can use it as a clean gate (ADR 0022).
(resolve 'no-such-var-xyz)
;; expect: nil
