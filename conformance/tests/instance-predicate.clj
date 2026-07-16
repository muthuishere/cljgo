;; instance? (ADR 0026): the class position is SYNTAX, matched by name (last
;; dotted segment against the designator table / TypeMarker), never resolved
;; as a var — same model as catch's class symbol. Works with cljgo designators
;; and JVM-spelled names alike.
;; oracle (clojure 1.12.5): all forms below produce the same booleans on the
;; JVM, where the symbols resolve to real classes.
[(instance? String "x")
 (instance? String 1)
 (instance? Long 1)
 (instance? Double 1.5)
 (instance? Boolean true)
 (instance? clojure.lang.Keyword :k)
 (instance? clojure.lang.Symbol 'sym)
 (instance? clojure.lang.Atom (atom 1))
 (instance? java.util.UUID (parse-uuid "c6e175b6-4b0c-4b6a-9dd8-b0e765a89a80"))
 (instance? java.util.UUID "not-a-uuid")]
;; expect: [true false true true true true true true true false]
