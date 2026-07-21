;; ADR 0040 #2 close semantics: a closed channel drains its buffered
;; values in order before yielding nil forever; a put AFTER close returns
;; false immediately; double-close is a no-op returning nil.
;; oracle (JVM core.async 1.6.681, spikes/s19-core-async/oracle/transcript.txt):
;;   closed-read-drains-buffer => [1 2 nil] · closed-put->!! => false ·
;;   double-close => nil
;; Deterministic: the size-2 buffer holds both puts before the close.
;; oracle: skip — needs the core.async dep; frozen from the S19 transcript
(def c (chan 2))
(>! c 1)
(>! c 2)
(def first-close (close! c))
(def second-close (close! c))
[(>! c 9) (<! c) (<! c) (<! c) first-close second-close]
;; expect: [false 1 2 nil nil nil]
