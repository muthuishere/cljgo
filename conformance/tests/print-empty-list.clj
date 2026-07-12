;; An empty list prints as (), including nested and when produced by
;; list/rest — the printer walks (seq coll), which is nil when empty,
;; instead of treating the empty collection as a one-element seq.
;; Expectation frozen from real Clojure 1.12.5 (clojure CLI, JDK 26), 2026-07-12:
;;   (pr-str ['() (list) '(()) (rest '(1))]) => "[() () (()) ()]"
[(quote ()) (list) (quote (())) (rest (quote (1)))]
;; expect: [() () (()) ()]
