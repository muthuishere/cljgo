;; The standard typed JVM exception classes are catchable, each matching
;; the core operation that throws it (ADR 0039 addendum). Oracle (clojure
;; 1.12.5, 2026-07-23, run file-for-file): (/ 1 0) throws
;; ArithmeticException; (+ 1 "a") and (compare 1 :a) ClassCastException;
;; (inc nil) NullPointerException; (nth [1 2] 5) IndexOutOfBoundsException;
;; (subs "ab" 5) StringIndexOutOfBoundsException (a subclass of
;; IndexOutOfBoundsException, so that catch matches); (first 5)
;; IllegalArgumentException; ((fn [x] x) 1 2) clojure.lang.ArityException;
;; (bigint ##Inf) NumberFormatException "Infinite or NaN".
[(try (/ 1 0) (catch ArithmeticException e :arith))
   (try (+ 1 "a") (catch ClassCastException e :cce))
   (try (compare 1 :a) (catch ClassCastException e :cce-cmp))
   (try (inc nil) (catch NullPointerException e :npe))
   (try (nth [1 2] 5) (catch IndexOutOfBoundsException e :ioobe))
   (try (subs "ab" 5) (catch IndexOutOfBoundsException e :str-ioobe))
   (try (first 5) (catch IllegalArgumentException e :iae))
   (try ((fn [x] x) 1 2) (catch clojure.lang.ArityException e :arity))
   (try (bigint ##Inf) (catch NumberFormatException e :nfe))
 (try (/ 1 0) (catch ArithmeticException e (ex-message e)))]
;; expect: [:arith :cce :cce-cmp :npe :ioobe :str-ioobe :iae :arity :nfe "Divide by zero"]
