;; ADR 0040 #1: buffer policies compose with a transducer — the xf runs
;; first, the policy governs the buffer add. dropping keeps the first
;; n transformed values; sliding keeps the last n.
;; oracle (JVM core.async 1.6.681, spikes/s19-core-async/oracle/transcript2.txt):
;;   dropping-with-xform => [1 2 nil] · sliding-with-xform => [4 5 nil]
;; Deterministic: single producer, then close+drain.
;; oracle: skip — needs the core.async dep; frozen from the S19 transcript
(def cd (chan (dropping-buffer 2) (map inc)))
(>! cd 0)
(>! cd 1)
(>! cd 2)
(>! cd 3)
(>! cd 4)
(close! cd)
(def cs (chan (sliding-buffer 2) (map inc)))
(>! cs 0)
(>! cs 1)
(>! cs 2)
(>! cs 3)
(>! cs 4)
(close! cs)
[(<! cd) (<! cd) (<! cd) (<! cs) (<! cs) (<! cs)]
;; expect: [1 2 nil 4 5 nil]
