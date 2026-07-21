;; ns-aliases / ns-imports (fundamentals audit 2026-07).
;; oracle (clojure 1.12.5, 2026-07-21) for the alias probes:
;;   after (require '[clojure.set :as sss]):
;;   (contains? (ns-aliases 'user) 'sss) => true
;;   (ns-name (get (ns-aliases 'user) 'sss)) => clojure.set
;; oracle: skip — the ns-imports probe is a documented cljgo DEVIATION:
;;   the Go host has no JVM class imports, so (ns-imports 'user) => {}
;;   where the JVM preloads java.lang (e.g. 'String -> java.lang.String).
;;   The alias probes DO hold verbatim on the JVM.
(require '[clojure.set :as sss])
[(contains? (ns-aliases 'user) 'sss)
 (ns-name (get (ns-aliases 'user) 'sss))
 (ns-imports 'user)]
;; expect: [true clojure.set {}]
