;; ADR 0040 #6: clojure.core.async is the CANONICAL namespace —
;; (require '[clojure.core.async :as async]) works, and the M4-v0
;; clojure.core names are refers of the SAME vars, so the alias and the
;; canonical name deref to the identical fn object. The namespace's fn
;; half is Go-native (interned at boot); the macro half (go-loop/alt!)
;; loads lazily on this require (core/async.cljg).
;; oracle (JVM core.async 1.6.681): (async/<!! (async/go 42)) => 42
;; (spikes/s19-core-async/oracle/transcript2.txt go-result => 42).
;; oracle: skip — needs the core.async dep; frozen from the S19 transcript
(require '[clojure.core.async :as async])
[(async/<!! (async/go 42)) (identical? chan async/chan) (identical? <! async/<!)]
;; expect: [42 true true]
