;; ADR 0040 #5: thread is an alias of go — a real goroutine returning a
;; result channel that delivers the body's value then closes. There is
;; no thread-pool/park distinction to preserve (no IOC transform).
;; oracle (JVM core.async 1.6.681, spikes/s19-core-async/oracle/transcript2.txt
;; + fresh 2026-07-21 run): thread-result => 42 ·
;; thread-drains-then-nil => [7 nil] · go-nil-result => nil
;; oracle: skip — needs the core.async dep; frozen from the S19 transcript
(def t (thread 7))
[(<! (thread 42)) (<! t) (<! t) (<! (go nil))]
;; expect: [42 7 nil nil]
