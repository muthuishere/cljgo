;; future-cancel (ADR 0038): cancelling a COMPLETED future => false; a
;; PENDING one => true, after which realized?/future-cancelled?/
;; future-done? are all true and deref throws (JVM CancellationException).
;; Cancellation is cooperative — the body goroutine is not interrupted
;; (suite realized_qmark.cljc).
;; oracle (clojure 1.12.5): [false true true true true :cancelled]
(let [done (future :x)
      _ @done
      p (promise)
      pending (future @p)]
  [(future-cancel done)
   (future-cancel pending)
   (realized? pending)
   (future-cancelled? pending)
   (future-done? pending)
   (try @pending (catch Exception _e :cancelled))])
;; expect: [false true true true true :cancelled]
