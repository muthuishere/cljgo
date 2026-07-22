;; ADR 0040 §2.4: (unsub p topic ch) stops delivery of that topic to ch. The
;; :b topic is a synchronization barrier: because the pub pump processes the
;; source in order, once the :b message arrives the earlier :a message has
;; already been routed — and dropped, since a-ch was unsubscribed.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(let [src (async/chan 10) p (async/pub src :tag) a-ch (async/chan 10) b-ch (async/chan 10)]
  (async/sub p :a a-ch) (async/sub p :b b-ch)
  (async/>!! src {:tag :a :n 1}) (async/<!! a-ch)
  (async/unsub p :a a-ch)
  (async/>!! src {:tag :a :n 2}) (async/>!! src {:tag :b :n 99}) (async/<!! b-ch)
  (async/poll! a-ch))
;; expect: nil
