;; The THROWING edges of the ADR 0067 second-table batch. What this file
;; freezes is that the unboxed emission (rt.IQuot/IRem/INeg on raw int64,
;; taken by a compiled binary inside a typed region) raises the SAME error
;; as the boxed tower path the interpreter takes — same class, same
;; message, eval == compiled.
;; DELIBERATE JVM WORDING DIVERGENCE, pre-dating this batch: cljgo's
;; int64Ops.Quotient/Remainder always say "/ by zero" and int64Ops.Negate
;; says "integer overflow". JVM 1.12.5 varies with call shape — verified
;; 2026-07-27: (quot 1 0) at an inlined site => "/ by zero", but the same
;; call through an unhinted fn arg (Numbers.quotient(Object,Object)) =>
;; "Divide by zero"; and (- Long/MIN_VALUE) => "long overflow". cljgo has
;; one wording per op regardless of call shape, which is why this file is a
;; cljgo-contract freeze rather than an oracle match.
;; oracle: cljgo eval harness (JVM divergence documented above).
(defn trunc-q [a b] (quot a b))
(defn trunc-r [a b] (rem a b))
(defn neg [x] (- x))
(defn dec-guard [x] (dec x))

[(try (trunc-q 1 0) (catch Throwable e (ex-message e)))
 (try (trunc-r 1 0) (catch Throwable e (ex-message e)))
 (try (neg -9223372036854775808) (catch Throwable e (ex-message e)))
 (try (dec-guard -9223372036854775808) (catch Throwable e (ex-message e)))
 (trunc-q -9223372036854775808 -1)
 (trunc-r -9223372036854775808 -1)]
;; expect: ["/ by zero" "/ by zero" "integer overflow" "integer overflow" -9223372036854775808 0]
