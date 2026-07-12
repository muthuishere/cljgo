;; Syntax-quote in a user macro: template symbols resolve against the
;; live ns world (reader Resolver), unquote/splice splice evaluated
;; args. Oracle (clojure 1.12.5): 11.
(defmacro add1 [x] `(+ 1 ~x))
(defmacro sum [& xs] `(+ ~@xs))
(+ (add1 3) (sum 1 2 4))
;; expect: 11
