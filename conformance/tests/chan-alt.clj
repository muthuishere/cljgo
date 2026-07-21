;; T1 (openspec core-async-first-class 1.4): alt!/alt!! — macros over
;; alts! (ADR 0040 #3; the AOT emission of a static Go `select` is a
;; recorded performance-ladder rung, not this change). Clause shapes:
;; take port with a plain result, ([v] ...) / ([v ch] ...) binding
;; results, a DOUBLY-wrapped [[c v]] put clause, a vector of ports
;; sharing one clause, :default and :priority.
;; oracle (fresh 2026-07-21 run, core.async 1.6.681 on Clojure 1.12.5):
;;   alt!-take-clause => :took · alt!-binding-clause => 42 ·
;;   alt!-put-clause-binding => [true true] · alt!-multi-port-vector => :b ·
;;   alt!-default-hit => :none · alt!-priority => :a
;; Deterministic: ready ports are pre-filled buffers; the :default case
;; runs against a channel that never becomes ready.
;; oracle: skip — needs the core.async dep; frozen from the fresh T1 oracle run
(require '[clojure.core.async :as async])
(def c1 (chan 1))
(>! c1 :v)
(def r-take (async/alt!! c1 :took :default :none))
(def c2 (chan 1))
(>! c2 41)
(def r-bind (async/alt!! c2 ([v] (inc v)) :default :none))
(def c3 (chan 1))
(def r-put (async/alt!! [[c3 :v]] ([res ch] [res (= ch c3)]) :default :none))
(def c4 (chan 1))
(def c5 (chan 1))
(>! c5 :b)
(def r-multi (async/alt!! [c4 c5] ([v] v) :default :none))
(def r-default (async/alt!! (chan) :took :default :none))
(def c6 (chan 1))
(def c7 (chan 1))
(>! c6 :a)
(>! c7 :b)
(def r-priority (async/alt!! c6 ([v] v) c7 ([v] v) :priority true))
[r-take r-bind r-put r-multi r-default r-priority]
;; expect: [:took 42 [true true] :b :none :a]
