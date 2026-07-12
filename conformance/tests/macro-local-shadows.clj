;; A local binding shadows a macro of the same name (Compiler.isMacro
;; consults the lexical env). Oracle (clojure 1.12.5): 2.
(let [when (fn [x y] y)]
  (when 1 2))
;; expect: 2
