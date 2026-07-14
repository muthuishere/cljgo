;; M4+ alts! (design/05 §4): wait on multiple take-ports via reflect.Select,
;; returning [val port] for the first ready one. Printing the port (a Channel)
;; is nondeterministic, so we assert on the VALUE only. The :default option is
;; non-blocking: with no ready port it returns [v :default]. Deterministic — a
;; buffered put is ready before the alts!, and an empty channel never becomes
;; ready so :default always wins.
;; oracle: skip — cljgo concurrency (JVM core.async differs)
(def c (chan 1))
(>! c 42)
(def r1 (first (alts! [c])))
(def d (chan))
(def r2 (alts! [d] :default :none))
[r1 (first r2) (second r2)]
;; expect: [42 :none :default]
