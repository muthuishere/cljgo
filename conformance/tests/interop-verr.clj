;; Go interop (ADR 0010, design/05 §2): the (T, error) shaping. A plain call
;; returns the [v err] vector; the happy err slot is nil-normalized; the error
;; branch passes the Go ZERO value through (Atoi's (0, err)) so eval and AOT
;; agree, and the error slot is a truthy value the program branches on
;; (errors-as-values). (get v 1) indexes the err slot — destructuring is a
;; later milestone.
;; oracle: skip — Go interop has no JVM Clojure equivalent
(require-go '[strconv])
[(strconv/Atoi "123")
 (first (strconv/Atoi "123"))
 (get (strconv/Atoi "123") 1)
 (first (strconv/Atoi "x"))
 (if (get (strconv/Atoi "x") 1) :parse-error :ok)]
;; expect: [[123 nil] 123 nil 0 :parse-error]
