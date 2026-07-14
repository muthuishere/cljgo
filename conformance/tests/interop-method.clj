;; Go interop (ADR 0010, design/05 §1): Clojure dot-form METHOD CALLS on a Go
;; object — `(.Method recv arg...)` => `recv.Method(args...)`. The receiver's
;; type is only known at runtime for M3.1, so BOTH modes call the method
;; reflectively (interpreter via reflect.MethodByName, AOT via rt.CallMethod
;; delegating to the SAME eval.CallGoMethod) — byte-identical by construction.
;; strings.NewReplacer returns a *strings.Replacer whose .Replace substitutes.
;; oracle: skip — Go interop has no JVM Clojure equivalent (Go stdlib is the oracle)
(require-go '[strings])
(def r (strings/NewReplacer "a" "1" "b" "2"))
[(.Replace r "abcab")
 (.Replace r "xyz")]
;; expect: ["12c12" "xyz"]
