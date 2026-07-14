;; M4 producer/consumer over an UNBUFFERED channel (design/05 §4): a real
;; goroutine produces three values, each >! rendezvousing with the main
;; goroutine's <!; then it closes. Output is deterministic — the unbuffered
;; handoff forces strict interleave — despite genuine concurrency. This is
;; the `cljgo run` vs `cljgo build` identical-output example (dual harness).
;; oracle: skip — cljgo concurrency (JVM core.async differs)
(def c (chan))
(go
  (>! c 1)
  (>! c 2)
  (>! c 3)
  (close! c))
[(<! c) (<! c) (<! c) (<! c)]
;; expect: [1 2 3 nil]
