;; The Reader Conditionals guide's two workarounds for the top-level
;; splicing restriction: one conditional per top-level defn, and a
;; single conditional wrapping multiple defns in a `do`. Both must
;; really define the fns.
;;
;; oracle: skip — :cljgo is cljgo's platform feature (ADR 0036). JVM
;; mirror verified 2026-07-23 (clojure 1.12.5, .cljc file with :clj):
;; `#?(:clj (defn f1 [] :abc))` then (f1) => :abc, and
;; `#?(:clj (do (defn g1 [] 1) (defn g2 [] 2)))` then [(g1) (g2)]
;; => [1 2].
#?(:cljgo (defn f1 [] :abc))
#?(:cljgo (do (defn g1 [] 1)
              (defn g2 [] 2)))
[(f1) (g1) (g2)]
;; expect: [:abc 1 2]
