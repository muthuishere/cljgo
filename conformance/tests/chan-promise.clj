;; T1 stragglers (openspec core-async-first-class 1.5): promise-chan is
;; a latch — the first put wins, EVERY take sees that value (takes do
;; not consume it), later puts are accepted-and-ignored (true); close!
;; without a value wakes takers with nil; a put after close returns
;; false.
;; oracle (JVM core.async 1.6.681, spikes/s19-core-async/oracle/transcript2.txt
;; + the fresh 2026-07-21 run): promise-chan-put-after-first => [:a :a] ·
;; promise-close-no-value => nil · promise-put-after-close => false ·
;; promise-offer-take => [true :v :v]
;; oracle: skip — needs the core.async dep; frozen from the S19 transcript
(require '[clojure.core.async :as async])
(def p (async/promise-chan))
(def put-a (>! p :a))
(def put-b (>! p :b))
(def takes [(<! p) (<! p) (async/poll! p)])
(def empty-p (async/promise-chan))
(close! empty-p)
[put-a put-b takes (<! empty-p) (>! empty-p :x)]
;; expect: [true true [:a :a :a] nil false]
