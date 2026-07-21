;; T1 (openspec core-async-first-class 1.5): go-loop —
;; (go (loop ...)) sugar; the JVM fuses them for its IOC transform, here
;; it is a plain macro over a real goroutine (core/async.cljg).
;; oracle (fresh 2026-07-21 run, core.async 1.6.681 on Clojure 1.12.5):
;;   go-loop-result => 3
;; Deterministic: the buffered puts land before the loop starts draining;
;; the closed channel ends the loop.
;; oracle: skip — needs the core.async dep; frozen from the fresh T1 oracle run
(require '[clojure.core.async :as async])
(def c (chan 3))
(>! c 1)
(>! c 2)
(close! c)
(async/<!! (async/go-loop [acc 0]
             (if-let [v (async/<! c)]
               (recur (+ acc v))
               acc)))
;; expect: 3
