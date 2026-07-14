;; Go interop (ADR 0010): a const in value position (math/Pi -> OpHostRef),
;; a float call, and Go number normalization — strconv.Atoi returns Go `int`
;; but both modes widen it to int64 so it prints as 456, not a host box.
;; oracle: skip — Go interop has no JVM Clojure equivalent
(require-go '[math])
(require-go '[strconv])
[math/Pi
 (math/Sqrt 16.0)
 (strconv/Itoa 42)
 (strconv/Atoi! "456")]
;; expect: [3.141592653589793 4.0 "42" 456]
