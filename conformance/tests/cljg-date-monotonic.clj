;; cljg.date (ADR 0101): nano-time is a MONOTONIC nanosecond clock — two
;; readings with work between them are non-decreasing ((<= t0 t1)), and `since`
;; reports the elapsed nanos (>= 0). now is wall-clock epoch millis (positive).
;; The exact nanos vary run to run, so this pins the monotonicity INVARIANTS,
;; not a wall value.
;;
;; oracle: skip — cljg.date is a cljgo host namespace (monotonic clock + wall
;; clock) with no stable JVM analog to freeze byte-for-byte; nano-time promotes
;; the same monotonic source the `time` macro's private -nano-time uses (the
;; System/nanoTime analog, verified non-decreasing). Frozen against cljgo's own
;; output; REPL-vs-binary parity is enforced by the dual harness.
(require '[cljg.date :as date])
(let [t0 (date/nano-time)
      _  (dotimes [_ 1000] (inc 1))
      t1 (date/nano-time)]
  [(<= t0 t1)
   (<= 0 (date/since t0 t1))
   (= (date/since t0 t1) (- t1 t0))
   (pos? (date/now))])
;; expect: [true true true true]
