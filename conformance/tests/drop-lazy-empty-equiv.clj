;; drop / drop-while on an already-exhausted seq must equal '() — NOT bare
;; nil. Real Clojure wraps both in `lazy-seq`, so the empty case returns a
;; LazySeq that seqs to nil; `=` treats a Sequential-whose-seq-is-nil as
;; equiv to '() (this is NOT the same as `(= nil '())`, which is false —
;; classic gotcha). An eager `nil` return (no lazy-seq wrap) breaks this.
;; Regression: clojure-test-suite core_test/drop.cljc + drop_while.cljc
;; (jank suite, ADR 0022).
;; Oracle (clojure 1.12.5): [true true]
[(= (quote ()) (drop-while pos? (quote ())))
 (= (quote ()) (drop 3 (list)))]
;; expect: [true true]
