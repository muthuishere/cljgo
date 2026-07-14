;; Go interop (ADR 0010, design/05 §1): struct CONSTRUCTION + field SET.
;; `(url/URL. {:Field v ...})` builds a *url.URL from a Clojure field map
;; (=> &url.URL{...}); `(set! (.-Field recv) v)` assigns a field
;; (=> recv.Field = v); `(go/new url/URL)` is a pointer to a zero-valued
;; struct (=> new(url.URL)). All three build reflectively in BOTH modes
;; (interpreter reflect.New + FieldByName.Set, AOT via rt.MakeStruct /
;; rt.FieldSet / rt.NewStruct delegating to the SAME shared eval fns) —
;; byte-identical by construction. z's fields are the type's zero values.
;; oracle: skip — Go interop has no JVM Clojure equivalent (Go stdlib is the oracle)
(require-go '[net/url])
(def u (url/URL. {:Scheme "https" :Host "x"}))
(set! (.-Host u) "y")
(def z (go/new url/URL))
[(.-Scheme u) (.-Host u) (.-Path z)]
;; expect: ["https" "y" ""]
