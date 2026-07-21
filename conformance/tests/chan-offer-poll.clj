;; T1 stragglers (openspec core-async-first-class 1.5): offer! is a
;; non-blocking put — true when accepted, nil (NOT false) when it would
;; block (both full-buffer and unbuffered-no-taker); poll! is a
;; non-blocking take — the value or nil. Both are clojure.core.async-only
;; names (nothing newer than M4-v0 lands in clojure.core — ADR 0040 #6).
;; oracle (JVM core.async 1.6.681, spikes/s19-core-async/oracle/transcript2.txt):
;;   offer-poll => [true nil 1 nil] · offer-on-unbuffered-no-taker => nil
;; oracle: skip — needs the core.async dep; frozen from the S19 transcript
(require '[clojure.core.async :as async])
(def c (chan 1))
[(async/offer! c 1) (async/offer! c 2) (async/poll! c) (async/poll! c)
 (async/offer! (chan) 1)]
;; expect: [true nil 1 nil nil]
