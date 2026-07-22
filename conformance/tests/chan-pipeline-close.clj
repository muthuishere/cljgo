;; ADR 0040 tier T3: pipeline RETURNS a completion channel that closes
;; (yields nil) once every result has been flushed to `to` — NOT `to`
;; itself (JVM core.async's pipeline* returns its final go-loop channel).
;; With close?=false, `to` is left OPEN when `from` drains, so after the
;; completion channel fires the three transformed values sit in `to` and a
;; further poll! sees nil (empty but open, not closed).
;; oracle (JVM core.async 1.6.681, Clojure 1.12.5, fresh 2026-07-22 run):
;;   [(<!! done) (<!! to) (<!! to) (<!! to) (poll! to)] => [nil 2 3 4 nil]
;; oracle: skip — needs the core.async dep; frozen from the run above
(require '[clojure.core.async :as async])
(let [to   (async/chan 100)
      done (async/pipeline 2 to (map inc) (async/to-chan! [1 2 3]) false)]
  [(async/<!! done)
   (async/<!! to) (async/<!! to) (async/<!! to) (async/poll! to)])
;; expect: [nil 2 3 4 nil]
