;; Go interop (ADR 0010, design/05 §1): stdlib string fns reached via
;; require-go as a Clojure namespace — single-return calls, int64->int
;; arg coercion (Repeat's count), both modes byte-identical.
;; oracle: skip — Go interop has no JVM Clojure equivalent (Go stdlib is the oracle)
(require-go '[strings])
[(strings/ToUpper "hi")
 (strings/ToLower "HI")
 (strings/Repeat "ab" 3)
 (strings/Contains "hello" "ell")
 (strings/HasPrefix "hello" "he")]
;; expect: ["HI" "hi" "ababab" true true]
