;; intern / create-ns / find-ns / var-get (design/08 batch E, ADR 0022):
;; intern creates-or-finds a Var in a namespace (2-arity leaves an
;; existing var's root untouched; 3-arity always sets it); an unknown
;; namespace symbol throws rather than being auto-created.
;; oracle (clojure 1.12.5): confirmed interactively via `clojure -e`.
[(let [x-var (intern (create-ns 'cljgo-test-intern-ns) 'x 42)]
   [(var-get x-var) (var-get (intern 'cljgo-test-intern-ns 'x))])
 (try (intern 'cljgo-conformance-unknown-ns-xyz 'x) :nothrow (catch Exception _e :threw))
 (some? (find-ns 'cljgo-test-intern-ns))
 (find-ns 'cljgo-conformance-totally-unknown-ns-abc)]
;; expect: [[42 42] :threw true nil]
