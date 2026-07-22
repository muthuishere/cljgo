;; ref history/tuning surface (fundamentals audit 2026-07):
;;   ensure            — protect a ref for the current transaction, returning
;;                       its in-transaction value; throws "No transaction
;;                       running" outside a dosync (like alter/ref-set).
;;   ref-history-count — 0: cljgo's STM-lite keeps no MVCC snapshot history
;;                       (single global lock), matching a fresh JVM ref.
;;   ref-min-history / ref-max-history — getter (arity 1) / setter (arity 2,
;;                       returns the ref). Defaults min 0, max 10; stored and
;;                       read back but not otherwise consulted.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   [10 0 0 10 true 2 true 20 "No transaction running"]
(let [r (ref 10)
      ensured (dosync (ensure r))
      hc (ref-history-count r)
      min0 (ref-min-history r)
      max0 (ref-max-history r)
      set-min-ret (identical? r (ref-min-history r 2))
      min1 (ref-min-history r)
      set-max-ret (identical? r (ref-max-history r 20))
      max1 (ref-max-history r)
      outside (try (ensure r) :no-throw (catch Exception e (ex-message e)))]
  [ensured hc min0 max0 set-min-ret min1 set-max-ret max1 outside])
;; expect: [10 0 0 10 true 2 true 20 "No transaction running"]
