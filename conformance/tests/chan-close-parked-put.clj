;; ADR 0040 #2 — THE close-rework fidelity proof: a put parked BEFORE
;; close! survives the close, is DELIVERED to a taker that arrives after
;; it, and returns true. M4-v0's shape (close the Go data chan + recover
;; the send panic) returned false and LOST the value. The data chan is
;; never closed now; a done chan signals closure (S19 gobacked2.go).
;; oracle (JVM core.async 1.6.681 on Clojure 1.12.5,
;; spikes/s19-core-async/oracle/probe3.txt):
;;   parked-put-survives-close => [:v true]
;; Deterministic up to the 50ms park window: the inner (>! c :v) parks on
;; the unbuffered channel; res is buffered so the outer put never blocks.
;; oracle: skip — needs the core.async dep; frozen from the S19 probe3 transcript
(def c (chan))
(def res (chan 1))
(go (>! res [:put-returned (>! c :v)]))
(<! (timeout 50))
(close! c)
[(<! c) (<! res) (<! c)]
;; expect: [:v [:put-returned true] nil]
