;; future / bound-fn (design/08 batch E, ADR 0022): future runs body in a
;; new goroutine, conveying the calling goroutine's dynamic-var bindings
;; (lang.AgentSubmit); bound-fn wraps a fn so that when INVOKED it
;; re-establishes only the vars that had an ACTIVE thread binding at wrap
;; time (get-thread-bindings — a var at just its root value, never
;; `binding`-shadowed yet, is not captured at all, so a later ambient
;; binding at call time still shows through: this is real Clojure's own
;; behavior, not a cljgo gap).
;; oracle (clojure 1.12.5): confirmed interactively via `clojure -e`.
(do
  (def ^:dynamic *bfx* :unset)
  [(deref (future (+ 1 2)))
   (let [f (bound-fn [] *bfx*)]
     [(f) (binding [*bfx* :set] (f))])
   (binding [*bfx* :caller]
     (let [f (future (binding [*bfx* :callee] (future (bound-fn [] *bfx*))))]
       (binding [*bfx* :derefer]
         ((deref (deref f))))))])
;; expect: [3 [:unset :set] :callee]
