;; Result/Option constructors (ADR 0014, spike S11): (ok v) (err e)
;; (just v) are calls; `none` is a VALUE (the shared sentinel). A nil
;; payload — (just nil) — stays distinct from none (type-per-tag nil
;; safety), and every value prints as a readable tagged literal.
;; oracle: skip — cljgo Result/Option primitive (no JVM ok/err/just/none)
[(ok 1) (err :boom) (just 5) none (just nil)]
;; expect: [#cljgo/ok 1 #cljgo/err :boom #cljgo/just 5 none #cljgo/just nil]
