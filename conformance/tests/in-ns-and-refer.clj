;; in-ns creates/switches the namespace; a bare namespace reaches core
;; only via qualified names until (clojure.core/refer 'clojure.core);
;; qualified syms resolve across namespaces (design/03 §7a, M1 exit).
;; Oracle (Clojure 1.12, 2026-07-12): same sequence (ns name scratch)
;;   → (+ user/x scratch/y) → 49.
(def x 42)
(in-ns 'm1.scratch)
(clojure.core/refer 'clojure.core)
(def y 7)
(in-ns 'user)
(+ user/x m1.scratch/y)
;; expect: 49
