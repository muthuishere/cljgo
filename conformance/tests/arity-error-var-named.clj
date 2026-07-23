;; A wrong-arity call to a var-bound single-fixed-arity fn names the VAR
;; (user/h) in the error — in BOTH legs: the interpreter's evalFn and a
;; compiled binary's *lang.NamedFnN wrapper raise the same named
;; ArityError (ADR 0048; before 2026-07-23 the compiled leg emitted a
;; bare lang.FnFuncN whose mismatch read "wrong number of arguments:
;; expected 1, got 3" — an unnamed REPL-vs-binary divergence).
;; oracle (clojure 1.12.5): (defn h [x] x)
;;   (try (h 1 2 3) (catch Throwable e (ex-message e)))
;;   => "Wrong number of args (3) passed to: user/h"
;; cljgo's frozen convention keeps lowercase "wrong" (see
;; conformance/tests/arity-error.clj).
(defn h [x] x)
(try (h 1 2 3) (catch Throwable e (ex-message e)))
;; expect: "wrong number of args (3) passed to: user/h"
