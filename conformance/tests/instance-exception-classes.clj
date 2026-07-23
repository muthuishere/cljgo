;; instance? answers the JVM exception ancestry for caught error values
;; (ADR 0039 addendum). Oracle (clojure 1.12.5, 2026-07-23, run
;; file-for-file): a caught ArithmeticException is an instance of itself,
;; RuntimeException, Exception and Throwable but NOT of a sibling class;
;; a NumberFormatException is an IllegalArgumentException (its JVM
;; superclass); an ArityException is an IllegalArgumentException; an
;; ex-info is a RuntimeException; a plain non-error value is no Throwable.
[(try (/ 1 0) (catch Throwable e
                [(instance? ArithmeticException e)
                 (instance? RuntimeException e)
                 (instance? Exception e)
                 (instance? Throwable e)
                 (instance? IllegalStateException e)]))
 (try (bigint ##Inf) (catch Throwable e
                       [(instance? NumberFormatException e)
                        (instance? IllegalArgumentException e)]))
 (try ((fn [x] x) 1 2) (catch Throwable e
                         [(instance? clojure.lang.ArityException e)
                          (instance? IllegalArgumentException e)
                          (instance? ArithmeticException e)]))
 (try (throw (ex-info "b" {})) (catch Throwable e
                                 [(instance? clojure.lang.ExceptionInfo e)
                                  (instance? RuntimeException e)]))
 (instance? Throwable "not an exception")]
;; expect: [[true true true true false] [true true] [true true false] [true true] false]
