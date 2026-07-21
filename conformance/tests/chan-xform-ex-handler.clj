;; ADR 0040 #1 ex-handler: a transducer-step exception is routed to the
;; ex-handler — a non-nil return is put IN PLACE of the poisoned value; a
;; nil return skips it; with NO handler the put still completes, the
;; poisoned value is dropped, and the channel stays usable (the observed
;; 1.6.681 behavior, frozen as conformance).
;; oracle (JVM core.async 1.6.681, spikes/s19-core-async/oracle/transcript2.txt):
;;   xform-ex-handler => :handled · xform-ex-handler-nil-return-skips => 1/2 ·
;;   xform-no-ex-handler-throws-where => :put-returned ·
;;   xform-no-exh-value-after => 1/2
;; oracle: skip — needs the core.async dep; frozen from the S19 transcript
(def ch (chan 1 (map (fn [x] (/ 1 x))) (fn [_] :handled)))
(>! ch 0)
(def r-handled (<! ch))
(def cn (chan 1 (map (fn [x] (/ 1 x))) (fn [_] nil)))
(>! cn 0)
(>! cn 2)
(def r-skip (<! cn))
(def cx (chan 1 (map (fn [x] (/ 1 x)))))
(def r-put (>! cx 0))
(>! cx 2)
(def r-after (<! cx))
[r-handled r-skip r-put r-after]
;; expect: [:handled 1/2 true 1/2]
