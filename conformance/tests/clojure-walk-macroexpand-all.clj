;; clojure.walk/macroexpand-all (core/walk.cljg): recursively expands every
;; macro call in a quoted form, through clojure.core/macroexpand. A
;; user-defined macro keeps the expansion host-independent (core macros may
;; legitimately expand differently per host).
;; oracle (clojure 1.12.5, 2026-07-21): with (defmacro twice [x] (list 'do x x)),
;;   (macroexpand-all '(twice (twice 1))) => (do (do 1 1) (do 1 1))
;; harness: eval — macroexpand is ADR 0046's bound-and-throwing stub in an
;; AOT-compiled binary; macroexpand-all resolves it per call and degrades
;; exactly the same way (requiring clojure.walk compiled still works).
(require '[clojure.walk :as w])
(defmacro twice [x] (list 'do x x))
(w/macroexpand-all '(twice (twice 1)))
;; expect: (do (do 1 1) (do 1 1))
