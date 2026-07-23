;; ns-unmap rejects a namespace-qualified symbol, exactly the JVM's
;; IllegalArgumentException message.
;; harness: eval — expect-error file (error path, REPL renderer)
;; oracle (Clojure 1.12.5 CLI, 2026-07-23): (ns-unmap *ns*
;; 'clojure.core/map) THREW "Can't unintern namespace-qualified symbol"
(ns-unmap *ns* 'clojure.core/map)
;; expect-error: Can't unintern namespace-qualified symbol
