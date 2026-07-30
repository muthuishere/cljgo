;; Call-heavy microbench (ADR 0064 cross-var direct calls).
;;
;; Every iteration makes three calls to small top-level defns of the SAME
;; compilation unit — the exact shape ADR 0064 targets. Before the change
;; each of those emitted `v_user_f.Get()` (an atomic.Value load + Box
;; unwrap) followed by `lang.ApplyN` (a type-switch dispatch on `any`);
;; after it, each is a guarded direct invocation of the def's published
;; lang.FnFuncN handle. The bodies are deliberately trivial so the
;; measurement is dominated by call overhead rather than by the work.
(defn id1 [x] x)
(defn add2 [a b] (+ a b))
(defn mix3 [a b c] (+ a (- b c)))

(defn work [n]
  (loop [i 0 acc 0]
    (if (< i n)
      (recur (inc i) (add2 (id1 acc) (mix3 1 i i)))
      acc)))

(work 10000000)
