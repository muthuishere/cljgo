;; binding makes its bindings in parallel: every init expression sees
;; the OLD bindings (unlike let*'s sequential scoping).
;; Oracle (Clojure 1.12, 2026-07-12):
;;   (binding [*a* 10 *b* *a*] [*a* *b*]) → [10 1].
(def ^:dynamic *a* 1)
(def ^:dynamic *b* 2)
(binding [*a* 10 *b* *a*] [*a* *b*])
;; expect: [10 1]
