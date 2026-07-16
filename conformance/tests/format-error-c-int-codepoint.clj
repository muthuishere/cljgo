;; format (ADR 0030, spike S14, section B. one conversion, one arg, no flags): c-int-codepoint throws.
;; Oracle: real `clojure` CLI 1.12.5, re-verified at freeze time (2026-07-16),
;; throws java.util.IllegalFormatConversionException (simple class name captured via
;; (.getSimpleName (class e)) in the oracle harness).
(format "%c" 65)
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; expect-error: IllegalFormatConversionException
