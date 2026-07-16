;; format (ADR 0030, spike S14, section K. error conditions): missing-arg throws.
;; Oracle: real `clojure` CLI 1.12.5, re-verified at freeze time (2026-07-16),
;; throws java.util.MissingFormatArgumentException (simple class name captured via
;; (.getSimpleName (class e)) in the oracle harness).
(format "%s %s" "only-one")
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; expect-error: MissingFormatArgumentException
