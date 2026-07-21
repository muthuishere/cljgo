;; T1 (openspec core-async-first-class 1.4): alts! write ports and the
;; done-case integration — a write [c v] port completes with [true c]; a
;; write to a CLOSED channel completes immediately with false; a CLOSED
;; read port counts as READY (yields nil), so :default is NOT taken.
;; oracle (JVM core.async 1.6.681: alts-write-op => [true ch] in the S19
;; transcript; fresh 2026-07-21 run: alts-write-closed => false,
;; alts-closed-ready-default => nil).
;; Ports print nondeterministically, so values only.
;; oracle: skip — needs the core.async dep; frozen from the oracle transcripts
(def c (chan 1))
(def r-write (first (alts! [[c :v]])))
(def taken (<! c))
(def closed (chan 1))
(close! closed)
(def r-closed-write (first (alts! [[closed :x]])))
(def r-closed-read (first (alts! [closed] :default :none)))
[r-write taken r-closed-write r-closed-read]
;; expect: [true :v false nil]
