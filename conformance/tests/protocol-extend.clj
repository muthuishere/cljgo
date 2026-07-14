;; extend-protocol extends a protocol to existing types after the fact,
;; including built-in types and nil. cljgo dispatches on Go value type, so
;; the built-in designators are String / Long / nil (the JVM oracle writes
;; java.lang.String / java.lang.Long / nil — same behavior, host-specific
;; class names). Verified vs Clojure CLI 1.12.5 (java.lang.* designators):
;;   => ["str:x" "long:7" "nada" false]
;; expect: ["str:x" "long:7" "nada" false]
(defprotocol Describe (describe [this]))

(extend-protocol Describe
  String (describe [this] (str "str:" this))
  Long (describe [this] (str "long:" this))
  nil (describe [this] "nada"))

[(describe "x") (describe 7) (describe nil) (satisfies? Describe :kw)]
