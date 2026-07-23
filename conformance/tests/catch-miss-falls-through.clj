;; A typed catch clause that does NOT match lets the throw propagate to
;; the enclosing try — never swallows it (ADR 0039 addendum). Oracle
;; (clojure 1.12.5, 2026-07-23, run file-for-file): an ArithmeticException
;; sails past catch IllegalStateException / ClassCastException clauses and
;; is caught by the outer matching clause; a sibling ClassCastException
;; clause on the same try is skipped while the matching one fires.
[(try
   (try (/ 1 0) (catch IllegalStateException e :inner-wrong))
   (catch ArithmeticException e :outer-arith))
 (try
   (try (inc nil) (catch ClassCastException e :inner-wrong))
   (catch NullPointerException e :outer-npe))
 (try (/ 1 0)
      (catch ClassCastException e :skipped)
      (catch ArithmeticException e :matched))]
;; expect: [:outer-arith :outer-npe :matched]
