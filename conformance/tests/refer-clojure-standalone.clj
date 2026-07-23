;; refer-clojure as a standalone macro: (refer 'clojure.core & filters).
;; After ns-unmap'ing a core name, a bare (refer-clojure) restores it.
;; oracle (Clojure 1.12.5 CLI, 2026-07-23): (ns-unmap *ns* 'map)
;; (refer-clojure) (some? (resolve 'map)) => true; expansion is
;; (clojure.core/refer (quote clojure.core) <filters>)
(ns-unmap *ns* 'map)
(refer-clojure)
(some? (resolve 'map))
;; expect: true
