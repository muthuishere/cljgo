;; set! on a non-assignable expression is an analysis-time error.
;; Oracle (Clojure 1.12, 2026-07-12): "Invalid assignment target".
(set! "notatarget" 3)
;; expect-error: invalid assignment target
