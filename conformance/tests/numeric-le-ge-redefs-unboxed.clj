;; with-redefs of <= / >= must be honored by compiled binaries through the
;; ADR 0067 unboxed comparison paths: <= and >= joined the sealed core set
;; (rt.Boot seals nine vars), so redefining either trips lang.CoreArithDirty,
;; every typed region's `!rt.CoreDirty()` entry guard falls through to the
;; boxed body, and rt.LE2/GE2/LEBool/GEBool deref the redefined var per call.
;; Shapes: fib (<=-guarded, specialized + rung-3 lifted; redefined <= makes
;; (fib 2) = -1 and still terminates) and ge10 (string body, unspecializable,
;; its >= test emits rt.GEBool in the boxed path; redefined >= flips the
;; branch). Pristine before/after calls prove the restore.
;; NOTE deliberate JVM divergence (ADR 0066 §context): JVM 1.12.5 :inline
;; comparisons do NOT see these redefs at compiled call sites — it yields
;; [55 "small" 1 "small" 55 "small"] (verified 2026-07-23). cljgo is strictly
;; MORE live (ADR 0004); eval and compiled must match byte-identically.
;; oracle: cljgo eval harness (JVM divergence documented above).
(defn fib [n] (if (<= n 1) n (+ (fib (- n 1)) (fib (- n 2)))))
(defn ge10 [n] (if (>= n 10) "big" "small"))
[(fib 10)
 (ge10 5)
 (with-redefs [<= (fn [a b] (< a b))] (fib 2))
 (with-redefs [>= (fn [a b] true)] (ge10 5))
 (fib 10)
 (ge10 5)]
;; expect: [55 "small" -1 "big" 55 "small"]
