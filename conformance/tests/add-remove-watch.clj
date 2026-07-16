;; add-watch / remove-watch (ADR 0022 batch/harness-misc): the watch fn is
;; called (key ref old new) on every swap!/reset!; add-watch returns the ref
;; itself; remove-watch detaches by key.
;; oracle (clojure 1.12.5):
;;   log after inc + reset + remove + inc => [[:k 0 1] [:k 1 10]], @a => 11,
;;   (identical? a (add-watch a ...)) => true.
[(let [a (atom 0)
       log (atom [])
       same (identical? a (add-watch a :k (fn [k r o n] (swap! log conj [k o n]))))]
   (swap! a inc)
   (reset! a 10)
   (remove-watch a :k)
   (swap! a inc)
   [same @log @a])]
;; expect: [[true [[:k 0 1] [:k 1 10]] 11]]
