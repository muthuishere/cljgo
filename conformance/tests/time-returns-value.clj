;; time (fundamentals batch 1): evaluates the expression, prns
;; "Elapsed time: <ms> msecs", and returns the expression's value. The
;; elapsed digits vary run to run, so the test pins the SHAPE of the
;; printed line (prn of a string => quoted + newline) and the value.
;; oracle (clojure 1.12.5): (with-out-str (time (+ 1 2))) =>
;; "\"Elapsed time: 0.004458 msecs\"\n" (digits vary; regex below
;; matches); value of (time (+ 40 2)) => 42.
(def out (with-out-str (time (+ 1 2))))
(def v (volatile! nil))
(def out2 (with-out-str (vreset! v (time (+ 40 2)))))
[(boolean (re-find #"^\"Elapsed time: .+ msecs\"\n$" out)) @v]
;; expect: [true 42]
