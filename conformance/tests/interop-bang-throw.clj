;; Go interop (ADR 0010, design/05 §2): the `!` suffix throws when the Go
;; call's trailing error is non-nil (the 90% path — unwrap-or-throw). Go
;; exports can never end in `!`, so the suffix is unambiguous sugar.
;; oracle: skip — Go interop has no JVM Clojure equivalent
;; harness: eval — the throw is a runtime panic; try/catch (dual-mode capture) lands in a later milestone
(require-go '[strconv])
(strconv/Atoi! "not-a-number")
;; expect-error: invalid syntax
