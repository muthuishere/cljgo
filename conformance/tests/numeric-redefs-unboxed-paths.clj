;; with-redefs of core arithmetic must be honored by compiled binaries even
;; through the ADR 0067 unboxed int64 fast paths: redefining + or * trips
;; the ADR 0066 sealed-core dirty flag (lang.CoreArithDirty), every typed
;; region's `!rt.CoreDirty()` entry guard then falls through to the boxed
;; body, and the boxed guarded intrinsics deref the redefined var per call.
;; Shapes covered: add1 (specialized + rung-3 lifted; redefined +), fact
;; (lifted self-recursive; redefined *), sum10 (0-param specialized fn
;; whose body is a typed loop; redefined +), sumstr (an UNspecializable fn
;; — string body — whose inner numeric loop is dual-emitted; redefined +),
;; plus pristine before/after calls proving the restore (the flag is
;; sticky, so the after-calls also prove the boxed path re-verifies the
;; restored root and takes the pristine tower again).
;; The + redef avoids calling + (which would self-recurse under redef) via
;; unchecked-add, and keeps loops terminating.
;; NOTE deliberate JVM divergence (ADR 0066 §context): JVM 1.12.5 :inline
;; arithmetic does NOT see these redefs at compiled call sites — it yields
;; [6 120 45 [6 45 "s=45"] 24 6 120 45]. cljgo is strictly MORE live (ADR
;; 0004 per-call deref in the tree-walk; the compiled binary must match the
;; tree-walk byte-identically — the dual-harness contract).
;; oracle: cljgo eval harness (JVM divergence documented above and in ADR 0066).
(defn add1 [n] (+ n 1))
(defn fact [n] (if (< n 2) 1 (* n (fact (- n 1)))))
(defn sum10 [] (loop [i 0 acc 0] (if (< i 10) (recur (+ i 1) (+ acc i)) acc)))
(defn sumstr [] (str "s=" (loop [i 0 acc 0] (if (< i 10) (recur (+ i 1) (+ acc i)) acc))))
[(add1 5)
 (fact 5)
 (sum10)
 (with-redefs [+ (fn [a b] (unchecked-add (unchecked-add a b) 1000))]
   [(add1 5) (sum10) (sumstr)])
 (with-redefs [* (fn [a b] (unchecked-add a b))]
   (fact 4))
 (add1 5)
 (fact 5)
 (sum10)]
;; expect: [6 120 45 [1006 1000 "s=1000"] 10 6 120 45]
