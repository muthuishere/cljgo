;; harness: standalone — probes (ns-publics 'user) membership of the short
;; names p/q, which a shared batch binary could collide with via sibling
;; programs' package-init interns; runs as its own binary for a clean registry.
;; ^:private survives compilation (regression, fundamentals audit 2026-07).
;; A compiled var is interned by NAME, so before pkg/emit carried :private
;; explicitly every private helper came back public in a binary: (dir
;; clojure.set) listed -bubble-max-key under `cljgo build` and not in the
;; REPL — a REPL-vs-binary divergence (CLAUDE.md's unforgivable failure
;; mode), caught by conformance/tests/repl-tooling.clj.
;; oracle (clojure 1.12.5, 2026-07-21): [(contains? (ns-publics *ns*) 'p)
;; (contains? (ns-publics *ns*) 'q) (:private (meta #'p))
;; (boolean (:private (meta #'q)))] => [false true true false]
(defn ^:private p [] 1)
(defn q [] 2)
[(contains? (ns-publics *ns*) 'p)
 (contains? (ns-publics *ns*) 'q)
 (:private (meta #'p))
 (boolean (:private (meta #'q)))]
;; expect: [false true true false]
