;; Overflow at the boundary of a `<=`-specialized lifted fn (ADR 0067): the
;; typed int64 fast path uses checked rt.IAdd, so the tower's overflow throw
;; survives unboxing — (big 1) adds (MAX-1)+(MAX-1) and must throw in BOTH
;; eval and compiled modes, while (big 0) returns the near-max value intact.
;; The catch freezes the THROW (a keyword), not the message: JVM 1.12.5 says
;; "long overflow" (Math.addExact) here while cljgo's tower standardizes
;; "integer overflow" (see numeric-overflow-throws.clj) — a pre-existing
;; message-text divergence, out of scope for this test.
;; oracle (clojure CLI 1.12.5, verified 2026-07-23): [9223372036854775806 :overflow].
(defn big [n] (if (<= n 0) 9223372036854775806 (+ (big (- n 1)) (big (- n 1)))))
[(big 0)
 (try (big 1) (catch Throwable e :overflow))]
;; expect: [9223372036854775806 :overflow]
