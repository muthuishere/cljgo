;; Result/Option combinators (ADR 0014): unwrap returns the ok/just
;; payload; unwrap-or falls back on err/none; map-ok maps the happy
;; payload (re-wrapping the same tag) and passes err/none through;
;; map-err maps only err; and-then is the railway bind — it feeds the
;; UNWRAPPED payload to the fn and short-circuits err/none unchanged.
;; oracle: skip — cljgo Result/Option primitive (no JVM ok/err/just/none)
[(unwrap (ok 42))
 (unwrap-or (err :x) 99)
 (map-ok inc (ok 1))
 (map-ok inc (err :e))
 (map-err (fn [e] :wrapped) (err :e))
 (and-then (fn [x] (ok (inc x))) (ok 1))
 (and-then (fn [x] (ok (inc x))) (err :stop))
 (and-then (fn [x] (ok (inc x))) none)]
;; expect: [42 99 #cljgo/ok 2 #cljgo/err :e #cljgo/err :wrapped #cljgo/ok 2 #cljgo/err :stop none]
