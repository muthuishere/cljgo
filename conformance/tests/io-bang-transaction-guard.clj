;; io! (fundamentals batch 1): runs its body (implicit do) outside a
;; transaction; inside a dosync it throws IllegalStateException before
;; the body runs. An optional leading string is the error message and is
;; NOT part of the body.
;; oracle (clojure 1.12.5): (io! (+ 1 2)) => 3; (dosync (io! (+ 1 2)))
;; => threw [java.lang.IllegalStateException "I/O in transaction"];
;; (dosync (io! "custom msg" ...)) => threw "custom msg";
;; (io! "custom msg" (+ 1 2)) => 3 (message dropped outside).
[(io! (+ 1 2))
 (try (dosync (io! (+ 1 2))) (catch Exception e (ex-message e)))
 (try (dosync (io! "custom msg" (+ 1 2))) (catch Exception e (ex-message e)))
 (io! "custom msg" (+ 1 2))]
;; expect: [3 "I/O in transaction" "custom msg" 3]
