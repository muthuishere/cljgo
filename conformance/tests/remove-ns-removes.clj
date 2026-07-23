;; remove-ns removes the named namespace and returns it (asserted via
;; some? — the Namespace's print form is host-specific); removing an
;; unknown namespace returns nil.
;; oracle (Clojure 1.12.5 CLI, 2026-07-23): (create-ns 'scratch.bb) then
;; [(some? (remove-ns 'scratch.bb)) (find-ns 'scratch.bb)
;; (remove-ns 'scratch.never)] => [true nil nil]
(create-ns 'scratch.bb)
[(some? (remove-ns 'scratch.bb)) (find-ns 'scratch.bb) (remove-ns 'scratch.never)]
;; expect: [true nil nil]
