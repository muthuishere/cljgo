;; cljg.system/sleep — the Thread/sleep analog (known-issues 2026-07-28 #10).
;; Blocks the calling thread for `ms` milliseconds and returns nil; a sleep of
;; 20 ms measurably advances the monotonic clock (cljg.date/nano-time). The
;; lower bound is deliberately slack (>= 10 ms of the 20 requested) so the file
;; is not timing-flaky under a loaded CI box — a sleep that returned instantly
;; still fails it.
;;
;; oracle: skip — cljg.system is a cljgo host namespace with no JVM package, so
;; this file cannot run through the `clojure` CLI verbatim. The contract was
;; verified against Thread/sleep at authoring time (clojure 1.12.5, 2026-07-28):
;;   (prn [(nil? (Thread/sleep 5))
;;         (let [t0 (System/nanoTime)] (Thread/sleep 50)
;;              (>= (- (System/nanoTime) t0) 40000000))])
;;   => [true true]
;; Frozen against cljgo's output; REPL-vs-binary parity is the dual harness's.
(require '[cljg.system :as sys])
(require '[cljg.date :as date])
(let [t0 (date/nano-time)
      r  (sys/sleep 20)]
  [(nil? r) (>= (date/since t0) 10000000)])
;; expect: [true true]
