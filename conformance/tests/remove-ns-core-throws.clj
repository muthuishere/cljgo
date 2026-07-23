;; remove-ns refuses to remove clojure.core, with the JVM's exact message.
;; harness: eval — expect-error file (error path, REPL renderer)
;; oracle (Clojure 1.12.5 CLI, 2026-07-23): (remove-ns 'clojure.core)
;; THREW "Cannot remove clojure namespace"
(remove-ns 'clojure.core)
;; expect-error: Cannot remove clojure namespace
