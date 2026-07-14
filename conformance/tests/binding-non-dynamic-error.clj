;; binding a var not marked ^:dynamic fails at runtime.
;; Oracle (Clojure 1.12, 2026-07-12): IllegalStateException
;;   "Can't dynamically bind non-dynamic var: user/plain".
(def plain 1)
(binding [plain 2] plain)
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; expect-error: bind non-dynamic var
