;; Go interop (ADR 0010, design/05 §1): Clojure dot-form FIELD ACCESS on a Go
;; object — `(.-Field recv)` => `recv.Field`. The receiver's type is only known
;; at runtime for M3.2, so BOTH modes read the field reflectively (interpreter
;; via reflect.FieldByName, AOT via rt.FieldGet delegating to the SAME
;; eval.GoFieldGet) — byte-identical by construction. url.Parse returns a
;; *url.URL whose exported fields Scheme/Host/Path carry the parsed components.
;; oracle: skip — Go interop has no JVM Clojure equivalent (Go stdlib is the oracle)
(require-go '[net/url])
(def u (url/Parse! "https://example.com/a/b"))
[(.-Scheme u) (.-Host u) (.-Path u)]
;; expect: ["https" "example.com" "/a/b"]
