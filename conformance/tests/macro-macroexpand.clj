;; macroexpand-1 / macroexpand as core fns (design/03 §4). Oracle
;; (clojure 1.12.5): (macroexpand '(when a b)) => (if a (do b)).
;; harness: eval — macroexpand/macroexpand-1 are the analyzer's expander
;; (design/03 §4); an AOT-compiled binary has no analyzer, so they throw
;; there rather than lie (ADR 0046 §5; ClojureScript answers this the same
;; way — macroexpansion is compile-time only). Deviation from the JVM,
;; documented. Interpreted behavior below is unchanged.
(defmacro unless [t e] (list 'if t nil e))
[(macroexpand-1 '(unless false 42)) (macroexpand '(when a b))]
;; expect: [(if false nil 42) (if a (do b))]
