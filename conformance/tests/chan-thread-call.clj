;; ADR 0040 #5 + core-async-audit 2026-07: (thread-call f) runs f on a real
;; goroutine and returns a channel that yields f's result once and then
;; closes; a nil result sends nothing (the channel just closes, so <! reads
;; nil). This is the public fn the `thread` macro is built on — the same
;; runtime seam (lang.Go) as the cljgo-internal go*. There is no
;; thread-pool/park distinction to preserve (no IOC transform, ADR 0040 #5).
;; oracle (JVM core.async 1.6.681, Clojure 1.12.5, fresh 2026-07-22 run):
;;   (thread-call (fn [] (* 6 7))) => 42 · (thread-call (fn [] nil)) => nil
;; oracle: skip — needs the core.async dep; frozen from the run above
(require '[clojure.core.async :as async])
[(async/<!! (async/thread-call (fn [] (* 6 7)))) (async/<!! (async/thread-call (fn [] nil)))]
;; expect: [42 nil]
