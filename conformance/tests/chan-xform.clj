;; ADR 0040 #1 transducer channels: (chan n xform) applies the xf on the
;; put side — map transforms, filter drops, mapcat expands, and a
;; `reduced` result (take's exhaustion) CLOSES the channel, after which
;; puts return false.
;; oracle (JVM core.async 1.6.681, spikes/s19-core-async/oracle/transcript.txt):
;;   xform-map => 2 · xform-filter-drops => 3 ·
;;   xform-mapcat-expansion => [1 1 1] · xform-reduced-closes => [1 2 nil false]
;; Deterministic: every put lands in buffer space before its take. The
;; mapcat channel gets buffer 3 (the S19 probe used 2): an expansion
;; larger than the FREE buffer applies backpressure mid-expansion here,
;; where the JVM completes it into a temporarily over-full buffer — the
;; S19-documented divergence (values identical, timing differs); a
;; single-goroutine file must therefore leave room for the expansion.
;; oracle: skip — needs the core.async dep; frozen from the S19 transcript
(def cm (chan 3 (map inc)))
(>! cm 1)
(def r-map (<! cm))
(def cf (chan 3 (filter odd?)))
(>! cf 2)
(>! cf 3)
(def r-filter (<! cf))
(def cc (chan 3 (mapcat (fn [x] [x x x]))))
(>! cc 1)
(def r-mapcat [(<! cc) (<! cc) (<! cc)])
(def ct (chan 5 (take 2)))
(>! ct 1)
(>! ct 2)
(def r-reduced [(<! ct) (<! ct) (<! ct) (>! ct 3)])
[r-map r-filter r-mapcat r-reduced]
;; expect: [2 3 [1 1 1] [1 2 nil false]]
