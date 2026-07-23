;; seque: a producer goroutine keeps the source realized ahead of the
;; consumer (default lookahead 100; explicit n in the 2-arity) — items
;; come back in source order, deterministically.
;; oracle (Clojure 1.12.5 CLI, 2026-07-23):
;; [(seque (range 10)) (seque 3 (map inc (range 5)))]
;; => [(0 1 2 3 4 5 6 7 8 9) (1 2 3 4 5)]
[(seque (range 10)) (seque 3 (map inc (range 5)))]
;; expect: [(0 1 2 3 4 5 6 7 8 9) (1 2 3 4 5)]
