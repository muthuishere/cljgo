;; ADR 0032 / spike S16: printing follows Java's toString — plain notation
;; iff scale >= 0 and adjusted exponent >= -6, else scientific with a signed
;; exponent. `str` omits the M suffix, `pr-str` keeps it; both preserve
;; scale (the old representation printed 1.10M as "1.1").
;; Oracle: real Clojure 1.12.5 CLI, verified 2026-07-16.
[(str 1.10M) (pr-str 1.10M)
 (str (bigdec "1e10")) (str 0.0000001M) (str 0.000001M)
 (pr-str [1.0M 1E+3M])]
;; expect: ["1.10" "1.10M" "1E+10" "1E-7" "0.000001" "[1.0M 1E+3M]"]
