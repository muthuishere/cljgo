;; Integer division by zero throws with the JVM's message text, and NEVER
;; leaks a Go runtime panic (fix 2026-07-22). Three bugs were here:
;;   * (/ 1 0) reached Go's big.NewRat(n, 0) and surfaced Go's own
;;     "division by zero" panic;
;;   * (/ 0 0) wrongly returned 0 instead of throwing;
;;   * (quot 1 0)/(rem 1 0) leaked Go's raw "runtime error: integer divide
;;     by zero", and the bigint quot/rem leaked math/big's "division by zero".
;;
;; JVM Clojure 1.12.5 messages (oracle, verified each separately):
;;   /            => "Divide by zero"   (all integer types)
;;   quot, rem    => "/ by zero"        (fixnum) · "Divide by zero" (bigint)
;;   mod          => "Divide by zero"
;; Float division by zero is a SEPARATE path and still yields ##Inf, as it must.
;;
;; DEVIATION (documented): (mod 1 0) on cljgo reports "/ by zero", not the
;; JVM's "Divide by zero". Both are the same divide-by-zero ArithmeticError;
;; the text differs ONLY because JVM's mod calls (rem num div) with boxed args
;; through Numbers.remainder ("Divide by zero") while a literal (rem 1 0)
;; inlines to primitive lrem ("/ by zero") — a JIT inlining artifact, not a
;; semantic difference. cljgo's mod faithfully propagates rem's message. The
;; bigint mod path matches the JVM exactly.
(let [msg (fn [f] (try (f) :NO-THROW (catch Exception e (ex-message e))))]
  [(msg #(/ 1 0)) (msg #(/ 0 0)) (msg #(/ 5 0)) (msg #(/ 10N 0N)) (msg #(/ 1/2 0))
   (msg #(quot 1 0)) (msg #(rem 1 0)) (msg #(quot 10N 0N)) (msg #(rem 10N 0N))
   (msg #(mod 10N 0N))
   (/ 1.0 0.0) (/ 1.0 0) (/ 1 0.0)])
;; expect: ["Divide by zero" "Divide by zero" "Divide by zero" "Divide by zero" "Divide by zero" "/ by zero" "/ by zero" "Divide by zero" "Divide by zero" "Divide by zero" ##Inf ##Inf ##Inf]
