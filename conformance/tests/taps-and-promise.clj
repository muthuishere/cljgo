;; add-tap/remove-tap/tap>/promise/deliver (design/08 batch E, ADR 0022):
;; every registered tap fn sees every tap>'d value (in tap> order) until
;; removed; add-tap/remove-tap return nil; tap> returns true; a promise
;; blocks deref until delivered, and delivering twice is a no-op.
;; cljgo dispatches tap> synchronously (real Clojure schedules async on
;; an agent executor) — functionally equivalent for a single-goroutine
;; caller like this test; not independently oracle-run (needs no
;; wall-clock coordination either way).
[(let [seen (atom [])
       t (fn [x] (swap! seen conj x))]
   [(add-tap t)
    (tap> :a)
    (tap> :b)
    (remove-tap t)
    (tap> :c)
    @seen])
 (let [p (promise)]
   [(realized? p)
    (some? (deliver p 42))
    (realized? p)
    (deref p)
    (some? (deliver p 99))
    (deref p)])]
;; expect: [[nil true true nil true [:a :b]] [false true true 42 false 42]]
