;; Doubles with magnitude >= 1e7 or < 1e-3 print in Java Double.toString
;; scientific notation: shortest round-trip digits, uppercase E, no '+'
;; or zero padding on the exponent, mantissa always d.ddd.
;; Expectation frozen from real Clojure 1.12.5 (clojure CLI, JDK 26), 2026-07-12:
;;   (pr-str [(- 23 5.647638473894739E258) 1e7 12345678.0 0.0001 1.0E100 -5.0E-9 1e16 (+ 0.1 0.2)])
;;   => "[-5.647638473894739E258 1.0E7 1.2345678E7 1.0E-4 1.0E100 -5.0E-9 1.0E16 0.30000000000000004]"
[(- 23 5.647638473894739E258) 1e7 12345678.0 0.0001 1.0E100 -5.0E-9 1e16 (+ 0.1 0.2)]
;; expect: [-5.647638473894739E258 1.0E7 1.2345678E7 1.0E-4 1.0E100 -5.0E-9 1.0E16 0.30000000000000004]
