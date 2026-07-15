;; NaN? on a non-number (ADR 0022, design/08 §5). Oracle (clojure 1.12.5):
;; NaN? is ^double-typed, so a non-Number arg (string, keyword, char, nil)
;; throws a ClassCastException — it does NOT coerce and return false.
;; (NaN? "x") => ClassCastException: class java.lang.String cannot be cast
;; to class java.lang.Number.
(NaN? "x")
;; expect-error: NaN?: not a number
