;; reduce over chunked sources (ADR 0045 continuation, 2026-07-22).
;;
;; cljgo's native reduce now takes a CHUNKED fast path when the source hands
;; out chunks (range, vector seqs): it walks a chunk by index and advances a
;; whole chunk at a time, instead of allocating one seq node per element.
;; That is how Clojure itself reduces chunked seqs, and how let-go gets its
;; edge on this workload (references/let-go pkg/rt/native_prims.go reduceColl).
;;
;; The observable semantics must not move, which is what this freezes — in
;; particular `reduced` still short-circuits and returns its value, even
;; though the walk now happens inside a chunk.
;; oracle (clojure 1.12.5, 2026-07-22):
;;   [(reduce + (range 10)) (reduce + 100 (range 10))
;;    (reduce (fn [a b] (if (> a 5) (reduced :stopped) (+ a b))) (range 100))
;;    (reduce + []) (reduce + [7]) (reduce conj [] (range 3))]
;;   => [45 145 :stopped 0 7 [0 1 2]]
[(reduce + (range 10))
 (reduce + 100 (range 10))
 (reduce (fn [a b] (if (> a 5) (reduced :stopped) (+ a b))) (range 100))
 (reduce + [])
 (reduce + [7])
 (reduce conj [] (range 3))]
;; expect: [45 145 :stopped 0 7 [0 1 2]]
