;; format (ADR 0030, spike S14, section C. uppercase variants (only b,h,s,c,x,e,g,a,t have them in Java)): D-invalid-no-upper throws.
;; Oracle: real `clojure` CLI 1.12.5, re-verified at freeze time (2026-07-16),
;; throws java.util.UnknownFormatConversionException (simple class name captured via
;; (.getSimpleName (class e)) in the oracle harness).
(format "%D" 1)
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; expect-error: UnknownFormatConversionException
