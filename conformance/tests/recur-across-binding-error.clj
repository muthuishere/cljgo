;; recur cannot cross a binding form: Clojure's binding expands to
;; try/finally around push/popThreadBindings, so this is the reserved
;; "cannot recur across try" (design/03 §2 Phase 4).
;; Oracle (Clojure 1.12, 2026-07-12): "Cannot recur across try".
(def ^:dynamic *d* 1)
(loop* [a 1] (binding [*d* 2] (recur 2)))
;; expect-error: recur across try
