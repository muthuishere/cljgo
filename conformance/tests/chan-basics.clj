;; M4 channels v0 (design/05 §4): buffered chan, >!/<!/close!, and the
;; closed+drained receive yielding nil. Deterministic despite the Go
;; channel underneath — a size-3 buffer holds all three puts before any
;; take. oracle: skip — cljgo concurrency, not JVM core.async semantics.
;; oracle: skip — cljgo channels (JVM core.async differs)
(def c (chan 3))
(>! c 10)
(>! c 20)
(>! c 30)
(close! c)
[(<! c) (<! c) (<! c) (<! c)]
;; expect: [10 20 30 nil]
