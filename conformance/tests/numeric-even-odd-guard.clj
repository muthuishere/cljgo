;; even?/odd? accept only integers (ADR 0029 cluster D, spike S13): any
;; non-integer — double, ratio, bigdec, ##Inf/##NaN, nil — throws
;; IllegalArgument "Argument must be an integer: <str of n>" exactly as JVM
;; clojure.core (##Inf strs as "Infinity"; nil strs as "", so that message
;; ends with the space).
;; oracle (clojure 1.12.5): expectation vector below, byte-identical.
[(even? 122N)
 (odd? -119)
 (even? -0)
 (try (even? 1.5) (catch Throwable e (ex-message e)))
 (try (odd? 1/2) (catch Throwable e (ex-message e)))
 (try (even? 0.2M) (catch Throwable e (ex-message e)))
 (try (even? ##Inf) (catch Throwable e (ex-message e)))
 (try (odd? ##NaN) (catch Throwable e (ex-message e)))
 (try (even? nil) (catch Throwable e (ex-message e)))]
;; expect: [true true true "Argument must be an integer: 1.5" "Argument must be an integer: 1/2" "Argument must be an integer: 0.2" "Argument must be an integer: Infinity" "Argument must be an integer: NaN" "Argument must be an integer: "]
