;; ADR 0032 follow-on: :rounding UNNECESSARY throws ArithmeticException
;; ("Rounding necessary") when the value actually needs rounding to fit
;; the requested precision (S16 probes_wp.clj row wp1 UNNECESSARY 1.5*1).
;; Oracle: real Clojure 1.12.5 CLI, verified 2026-07-16 — "Execution error
;; (ArithmeticException) at ... Rounding necessary".
(with-precision 1 :rounding UNNECESSARY (* 1.5M 1M))
;; expect-error: Rounding necessary
