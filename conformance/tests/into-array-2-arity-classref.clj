;; 2-arg into-array (ADR 0036 follow-on): the first arg is a type hint —
;; cljgo has no real Class value (ADR 0025 arrays are Go slices), so the
;; hint is one of ADR 0036's interned ClassRefs (Long, Integer, Double,
;; Float, Boolean, Object, …) rather than a JVM primitive-TYPE field
;; access (`Long/TYPE` does not resolve — no static-field form). Maps to
;; the matching Go slice kind, mirroring the 1-arg typed ctors
;; (int-array/long-array/etc). Unblocks clojure-test-suite's reduce.cljc
;; ":cljgo" branch (bare class name in the interop map's type slots,
;; docs/suite-upstream.md) past its 2-arity into-array gap. (Integer and
;; Float need (int …)/(float …) casts here — real Clojure's boxed
;; reference-class arrays require an exact-type element, unlike a
;; primitive TYPE array's automatic widen/unbox; oracle-verified
;; 2026-07-17: `(into-array Integer [1 2 3])` throws "array element type
;; mismatch" on real Clojure since int literals are Long, not Integer.)
;; oracle (clojure 1.12.5, 2026-07-17):
;; "[[1 2 3] [1 2 3] [1.0 2.0] [1.0 2.0] [true false] [1 \"a\" :k] 4950 true]"
;;
;; harness: eval — class refs resolve at the interpreter's symbol-
;; resolution level (ADR 0036); AOT emission of bare class-name symbols
;; is deferred (the suite runs interpreted, ADR 0022 decision 4).
(pr-str [(vec (into-array Integer (map int [1 2 3])))
         (vec (into-array Long [1 2 3]))
         (vec (into-array Float (map float [1.0 2.0])))
         (vec (into-array Double [1.0 2.0]))
         (vec (into-array Boolean [true false]))
         (vec (into-array Object [1 "a" :k]))
         (reduce + (into-array Long (range 1 100)))
         (reduce #(and %1 %2) (into-array Boolean (repeat 5 true)))])
;; expect: "[[1 2 3] [1 2 3] [1.0 2.0] [1.0 2.0] [true false] [1 \"a\" :k] 4950 true]"
