;; M4+ timeout (design/05 §4): (timeout ms) is a channel that auto-closes after
;; ms milliseconds, so (<! (timeout ms)) blocks ~ms then yields nil (the
;; closed+drained receive). Deterministic result value (a closed channel always
;; yields nil) despite the wall-clock delay; the second form proves a timeout
;; channel composes as a normal closed channel.
;; oracle: skip — cljgo concurrency (JVM core.async differs)
[(<! (timeout 20)) (let [t (timeout 5)] (<! t) :done)]
;; expect: [nil :done]
