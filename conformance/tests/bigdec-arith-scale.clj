;; ADR 0032 / spike S16: Java's arithmetic scale rules — add/sub scale =
;; max(sx, sy), mul scale = sx + sy — with exact decimal values (the old
;; big.Float backing returned 3.3000000000000000002M for (+ 1.1M 2.2M)).
;; Oracle: real Clojure 1.12.5 CLI, verified 2026-07-16.
[(+ 1.10M 2.2M) (+ 1M 2M) (+ 1.1M 2.2M) (+ 1E+2M 1M)
 (- 5.00M 1.0M) (- 1.0M 1.0M)
 (* 1.10M 2.0M) (* 1.5M 1.5M) (* 1.10M 1M) (* 1E+2M 1E+3M)
 (*' 1.10M 2.0M) (+' 1.10M 2.2M)
 (inc 1.0M) (dec 1.00M) (- 2.0M) (abs -1.50M)
 (max 1.0M 2M) (min 1.0M 2M)]
;; expect: [3.30M 3M 3.3M 101M 4.00M 0.0M 2.200M 2.25M 1.10M 1E+5M 2.200M 3.30M 2.0M 0.00M -2.0M 1.50M 2M 1.0M]
