;; *err* / *command-line-args* / *flush-on-newline* (fundamentals batch
;; A1): *err* is bound (os.Stderr on the Go host), *command-line-args*
;; roots to nil when no args follow the program (cmd/cljgo `run` and the
;; emitted func main() both bind it from the trailing args — the
;; clojure.main contract), and *flush-on-newline* roots to true.
;; oracle (clojure 1.12.5, 2026-07-23): [true nil true]
[(some? *err*) *command-line-args* *flush-on-newline*]
;; expect: [true nil true]
