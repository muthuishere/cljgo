;; extend-protocol / extend-type accept the FULLY QUALIFIED class name a Java
;; arrival types (java.lang.String), not just the bare one — the qualified
;; spelling keys the same dispatch table as the bare one, exactly as the
;; functional `extend` already did for a class-ref VALUE.
;; Oracle (real clojure CLI 1.12.5, `clojure -M o4.clj`, 2026-07-28):
;;   s:hi / n:7 / b:true / true / true / d:1.5  =>
;;   ["s:hi" "n:7" "b:true" true true "d:1.5"]
;; expect: ["s:hi" "n:7" "b:true" true true "d:1.5"]
(defprotocol Describe (describe [x]))

(extend-protocol Describe
  java.lang.String  (describe [x] (str "s:" x))
  java.lang.Long    (describe [x] (str "n:" x))
  java.lang.Boolean (describe [x] (str "b:" x)))

(extend-type java.lang.Double
  Describe
  (describe [x] (str "d:" x)))

[(describe "hi")
 (describe 7)
 (describe true)
 (extends? Describe java.lang.String)
 (satisfies? Describe "hi")
 (describe 1.5)]
