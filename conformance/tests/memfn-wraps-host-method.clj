;; memfn (fundamentals batch 1): wrap a host method as a first-class fn
;; — (memfn Name) => (fn [target] (.Name target)); extra names become
;; extra fn params. Method names are the HOST's (Go-exported here, java
;; camelCase on the JVM), so the receiver/method differ but the macro is
;; the oracle's.
;; oracle: skip — Go method names; JVM analog verified (clojure 1.12.5):
;; (macroexpand-1 '(memfn getName)) => (fn [target263] (. target263
;; (getName))); (map (memfn intValue) [1 2 3]) => (1 2 3);
;; ((memfn charAt i) "abc" 1) => \b.
[((memfn Name) (symbol "a/b"))
 (vec (map (memfn Name) [(symbol "x/one") (symbol "y/two")]))
 ((memfn Namespace) (symbol "x/one"))]
;; expect: ["b" ["one" "two"] "x"]
