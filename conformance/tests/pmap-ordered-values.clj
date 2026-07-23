;; pmap: parallel map (goroutine futures), results deref'd IN ORDER —
;; deterministic values regardless of scheduling.
;; oracle (Clojure 1.12.5 CLI, 2026-07-23): (pmap inc (range 10)) =>
;; (1 2 3 4 5 6 7 8 9 10)
(pmap inc (range 10))
;; expect: (1 2 3 4 5 6 7 8 9 10)
