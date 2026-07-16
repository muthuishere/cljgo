;; *print-length* (ADR 0022 batch/harness-misc, PROVENANCE.md): when bound to
;; N, pr-str emits at most N elements/entries of a seq/vector/map followed by
;; "..."; nil (the root default) is unlimited. Set output is order-dependent
;; in both hosts, so sets are exercised by count only.
;; oracle (clojure 1.12.5):
;;   (binding [*print-length* 3] (pr-str '(0 1 2 3 4 5))) => "(0 1 2 ...)"
;;   (binding [*print-length* 3] (pr-str [1 2 3 4 5])) => "[1 2 3 ...]"
;;   (binding [*print-length* 1] (pr-str {:a 1 :b 2})) => "{:a 1, ...}"
;;   (binding [*print-length* 0] (pr-str '(1 2))) => "(...)"
;;   unbound => "[1 2 3 4 5]"
[(binding [*print-length* 3] (pr-str '(0 1 2 3 4 5)))
 (binding [*print-length* 3] (pr-str [1 2 3 4 5]))
 (binding [*print-length* 1] (pr-str {:a 1 :b 2}))
 (binding [*print-length* 0] (pr-str '(1 2)))
 (pr-str [1 2 3 4 5])]
;; expect: ["(0 1 2 ...)" "[1 2 3 ...]" "{:a 1, ...}" "(...)" "[1 2 3 4 5]"]
