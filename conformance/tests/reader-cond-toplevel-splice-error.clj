;; A splicing reader conditional at the TOP LEVEL is a reader error —
;; there is no surrounding form to splice into (Reader Conditionals
;; guide). Diagnostic R1010.
;;
;; oracle: skip — JVM equivalent verified 2026-07-23 (clojure 1.12.5):
;; a .cljc file containing `#?@(:clj [(defn f [] 1) (defn g [] 2)])`
;; fails to load with "Reader conditional splicing not allowed at the
;; top level."; likewise (read-string {:read-cond :allow}
;; "#?@(:clj [1 2])") throws the same message. cljgo's message is
;; byte-identical (the branch key here is :cljgo, cljgo's platform
;; feature per ADR 0036).
;; harness: eval — expect-error file; the compiled leg was manually
;; verified to reject the same form with the same message (same reader).
#?@(:cljgo [(defn f [] 1) (defn g [] 2)])
;; expect-error: Reader conditional splicing not allowed at the top level.
