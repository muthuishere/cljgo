;; ADR 0040 §2.4: (unsub-all p topic) removes every subscriber of a topic.
;; Both :a subscribers get the first message, then unsub-all clears them; a
;; :b sync subscriber confirms the second :a message was routed-and-dropped.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(let [src (async/chan 10) p (async/pub src :tag) a-ch (async/chan 10) b-ch (async/chan 10) s-ch (async/chan 10)]
  (async/sub p :a a-ch) (async/sub p :a b-ch) (async/sub p :b s-ch)
  (async/>!! src {:tag :a :n 1}) (async/<!! a-ch) (async/<!! b-ch)
  (async/unsub-all p :a)
  (async/>!! src {:tag :a :n 2}) (async/>!! src {:tag :b :n 99}) (async/<!! s-ch)
  [(async/poll! a-ch) (async/poll! b-ch)])
;; expect: [nil nil]
