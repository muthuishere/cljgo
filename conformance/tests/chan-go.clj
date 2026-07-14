;; M4 `go` round-trip (design/05 §4): (go body) runs body in a REAL
;; goroutine and returns a result channel that receives the body's value
;; then closes, so (<! (go ...)) composes. Deterministic: <! blocks until
;; the goroutine delivers 3.
;; oracle: skip — cljgo concurrency (JVM core.async differs)
(<! (go (+ 1 2)))
;; expect: 3
