;; <= and >= as ADR 0067 inferred comparisons: a `<=`-guarded self-recursive
;; fn (the benchmark-corpus fib shape) and a `>=`-guarded one both specialize
;; and lift to typed int64 funcs with the test emitted as a raw Go compare;
;; a `<=` loop test dual-emits; mixed int/double operands and chained 3+-arg
;; calls stay on the boxed tower/variadic paths. Values must be identical in
;; eval and compiled modes.
;; oracle (clojure CLI 1.12.5, verified 2026-07-23): byte-identical vector.
(defn fib [n] (if (<= n 1) n (+ (fib (- n 1)) (fib (- n 2)))))
(defn gauss [n] (if (>= 1 n) n (+ n (gauss (- n 1)))))
[(fib 10)
 (gauss 10)
 (loop [i 1 acc 0] (if (<= i 10) (recur (inc i) (+ acc i)) acc))
 (<= 1 1.5)
 (>= 2.0 3)
 (<= 1 2 2 3)
 (>= 3 2 2 1)
 (<= 2 1)
 (>= 2 3)]
;; expect: [55 55 55 true false true true false false]
