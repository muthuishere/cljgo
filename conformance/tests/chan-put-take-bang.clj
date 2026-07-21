;; T1 stragglers (openspec core-async-first-class 1.5): put! returns
;; true immediately when the channel is open (before any taker — the
;; delivery is asynchronous); its callback receives the completed put's
;; boolean (false when already closed); take! delivers the taken value
;; to its callback from a goroutine. Callbacks report back through
;; buffered channels, keeping the file deterministic.
;; oracle (JVM core.async 1.6.681, spikes/s19-core-async/oracle/transcript2.txt
;; + the fresh 2026-07-21 run): put!-returns-before-taker => true ·
;; take!-callback => :v · put!-cb-on-closed => [false false] ·
;; take!-on-closed => [:got nil]
;; oracle: skip — needs the core.async dep; frozen from the S19 transcript
(require '[clojure.core.async :as async])
(def c (chan 1))
(def r-put (async/put! c :v))
(def took (chan 1))
(async/take! c (fn [v] (>! took [:got v])))
(def r-take (<! took))
(def closed (chan))
(close! closed)
(def cb (chan 1))
(def r-closed-put (async/put! closed :x (fn [ok] (>! cb [:cb ok]))))
(def r-closed-cb (<! cb))
(def took2 (chan 1))
(async/take! closed (fn [v] (>! took2 [:got v])))
[r-put r-take r-closed-put r-closed-cb (<! took2)]
;; expect: [true [:got :v] false [:cb false] [:got nil]]
