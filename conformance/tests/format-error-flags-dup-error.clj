;; format (ADR 0030, spike S14, section D. flags on numeric conversions): flags-dup-error throws.
;; Oracle: real `clojure` CLI 1.12.5, re-verified at freeze time (2026-07-16),
;; throws java.util.DuplicateFormatFlagsException (simple class name captured via
;; (.getSimpleName (class e)) in the oracle harness).
(format "%--d" 1)
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; expect-error: DuplicateFormatFlagsException
