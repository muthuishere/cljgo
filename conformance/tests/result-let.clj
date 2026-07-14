;; let? — railway binding (ADR 0014 D5). Bindings run left to right: an
;; ok/just binds its UNWRAPPED payload, a plain value binds unchanged,
;; and the FIRST err/none short-circuits the whole form to that value
;; (so `b (err :stop)` skips `c` entirely and returns #cljgo/err :stop).
;; oracle: skip — cljgo Result/Option primitive (no JVM let? / ok/err)
[(let? [a (ok 1) b (ok 2)] (+ a b))
 (let? [a (ok 1) b (err :stop) c (ok 3)] (+ a b c))
 (let? [a (just 10) b 5] (+ a b))
 (let? [a (ok 1) b none] (+ a b))]
;; expect: [3 #cljgo/err :stop 15 none]
