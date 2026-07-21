;; clojure.repl (fundamentals audit 2026-07): the REPL tooling namespace —
;; demunge (host-mangled name back to a Clojure name), root-cause (walk the
;; cause chain to the bottom), apropos (name search across loaded
;; namespaces), dir-fn (a namespace's public names, sorted).
;; oracle (clojure 1.12.5, 2026-07-21): the same seven forms printed
;;   ["clojure.core/map" "my-ns/foo!" "inner" true true
;;    [difference index intersection]]
;; (apropos on a name nothing matches returns an EMPTY seq, not nil — hence
;; `empty?` rather than a truthiness check, which would pass either way.)
;; source-fn is deliberately absent here: cljgo retains no source text, so
;; it returns nil where the JVM returns a form. See repl-source-fn.clj.
(require '[clojure.repl :as r] 'clojure.string 'clojure.set)
[(r/demunge "clojure.core$map")
 (r/demunge "my_ns$foo_BANG_")
 (ex-message (r/root-cause (ex-info "outer" {} (ex-info "inner" {}))))
 (boolean (some #{'clojure.core/reduce} (r/apropos "reduce")))
 (empty? (r/apropos "zzz-no-such-name"))
 (vec (take 3 (r/dir-fn 'clojure.set)))]
;; expect: ["clojure.core/map" "my-ns/foo!" "inner" true true [difference index intersection]]
