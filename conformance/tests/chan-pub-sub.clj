;; ADR 0040 §2.4: (pub ch topic-fn) routes each source value to a per-topic
;; mult keyed by (topic-fn v); (sub p topic ch) subscribes. Closing the
;; source closes each topic channel and thus the subscribers, so the drains
;; terminate. Messages whose topic has no subscriber are dropped.
;; oracle: skip — needs the core.async dep; frozen from a JVM core.async 1.6.681 (Clojure 1.12.5) run, 2026-07-22
(require '[clojure.core.async :as async])
(let [src (async/chan 10) p (async/pub src :tag) a-ch (async/chan 10) b-ch (async/chan 10)]
  (async/sub p :a a-ch) (async/sub p :b b-ch)
  (async/onto-chan! src [{:tag :z :n 0} {:tag :a :n 1} {:tag :b :n 2} {:tag :a :n 3}])
  [(mapv :n (async/<!! (async/into [] a-ch))) (mapv :n (async/<!! (async/into [] b-ch)))])
;; expect: [[1 3] [2]]
