;; Catch matching honors the real JVM exception ancestry (ADR 0039
;; addendum). Oracle (clojure 1.12.5, 2026-07-23, superclass chain read off
;; .getSuperclass and every expression below run file-for-file):
;; ArithmeticException < RuntimeException < Exception < Throwable;
;; NumberFormatException < IllegalArgumentException;
;; clojure.lang.ArityException < IllegalArgumentException;
;; clojure.lang.ExceptionInfo < RuntimeException. The FIRST matching
;; clause wins, and a superclass clause listed first shadows a later
;; exact one.
[(try (/ 1 0) (catch RuntimeException e :runtime))
 (try (/ 1 0) (catch Exception e :exception))
 (try (/ 1 0) (catch Throwable e :throwable))
 (try (bigint ##Inf) (catch IllegalArgumentException e :iae-catches-nfe))
 (try ((fn [x] x) 1 2) (catch IllegalArgumentException e :iae-catches-arity))
 (try (throw (ex-info "b" {})) (catch RuntimeException e :re-catches-exinfo))
 (try (/ 1 0) (catch RuntimeException e :first) (catch ArithmeticException e :second))
 (try (throw (ex-info "b" {})) (catch ArithmeticException e :wrong) (catch clojure.lang.ExceptionInfo e :exinfo))]
;; expect: [:runtime :exception :throwable :iae-catches-nfe :iae-catches-arity :re-catches-exinfo :first :exinfo]
