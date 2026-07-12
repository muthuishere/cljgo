;; Doubles with 1e-3 <= magnitude < 1e7 print as plain decimal, always
;; with a '.' and at least one fractional digit; -0.0 keeps its sign.
;; Boundary cases 9999999.9 and 0.001 stay plain (only >= 1e7 / < 1e-3 go
;; scientific). Expectation frozen from real Clojure 1.12.5 (clojure CLI,
;; JDK 26), 2026-07-12:
;;   (pr-str [21.5 1.0 100000.0 9999999.9 0.001 0.0 -0.0 (/ 2.0 3.0)])
;;   => "[21.5 1.0 100000.0 9999999.9 0.001 0.0 -0.0 0.6666666666666666]"
[21.5 1.0 100000.0 9999999.9 0.001 0.0 -0.0 (/ 2.0 3.0)]
;; expect: [21.5 1.0 100000.0 9999999.9 0.001 0.0 -0.0 0.6666666666666666]
