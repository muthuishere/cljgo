;; ns-unmap removes a mapping (here a referred clojure.core var) from the
;; namespace: afterwards the name no longer resolves. Unmapping mutates
;; only the mapping table — direct var access (clojure.core/map) is
;; untouched.
;; oracle (Clojure 1.12.5 CLI, 2026-07-23): (ns-unmap *ns* 'map) => nil;
;; (resolve 'map) => nil
(ns-unmap *ns* 'map)
(resolve 'map)
;; expect: nil
