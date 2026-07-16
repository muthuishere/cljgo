;; STM-lite (ADR 0038): ref/dosync/alter/ref-set, agents with serialized
;; send + await, ref watches. alter outside a transaction throws
;; "No transaction running" (JVM IllegalStateException); nested dosync
;; joins the outer transaction (suite remove_watch.cljc).
;; oracle (clojure 1.12.5): [6 :no-tx 7 5 [[:k 0 1]]]
(let [no-tx (try (alter (ref 0) inc) (catch Exception _e :no-tx))
      nested (dosync (dosync (alter (ref 5) + 2)))
      a (agent 0)
      _ (send a + 2)
      _ (send a + 3)
      _ (await a)
      watched (ref 0)
      log (atom [])]
  (add-watch watched :k (fn [k _r o n] (swap! log conj [k o n])))
  [(dosync (alter (ref 1) + 5)) no-tx nested @a (do (dosync (alter watched inc)) @log)])
;; expect: [6 :no-tx 7 5 [[:k 0 1]]]
