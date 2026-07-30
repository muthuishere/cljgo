;; An arity error that unwinds through an OUTER call keeps the name of the
;; fn that actually mismatched — the outer callee is not blamed. Here the
;; lazy seq (map h [1] [2] [3]) is realized inside `count`, so the mismatch
;; on `h` is raised deep under `count`'s frame; the error still names user/h.
;; Before 2026-07-28 the interpreter's call-site enrichment re-labelled ANY
;; arity error unwinding through a call with THAT call's callee, so
;; `cljgo run` rendered "passed to: clojure.core/println" for
;; (println (map h [1] [2] [3])) while `cljgo build` and the compiled binary
;; both said user/h — a REPL-vs-binary divergence in error text
;; (docs/known-issues-2026-07-28.md §8).
;; oracle (clojure 1.12.5):
;;   (defn h [x] x)
;;   (try (count (map h [1] [2] [3])) (catch Throwable e (ex-message e)))
;;   => "Wrong number of args (3) passed to: user/h"
;; cljgo's frozen convention keeps lowercase "wrong" (see
;; conformance/tests/arity-error.clj).
(defn h [x] x)
(try (count (map h [1] [2] [3])) (catch Throwable e (ex-message e)))
;; expect: "wrong number of args (3) passed to: user/h"
