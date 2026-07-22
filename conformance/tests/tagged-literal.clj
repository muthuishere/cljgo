;; clojure.core/tagged-literal + tagged-literal? (ADR 0050).
;; A TaggedLiteral is the value a data reader receives: a :tag symbol and a
;; :form. It prints readably as `#tag form`, supports keyword lookup and =.
;;
;; oracle: JVM Clojure 1.12.5 (clojure CLI), 2026-07-22:
;;   (prn [(pr-str (tagged-literal 'foo 42))
;;         (:tag (tagged-literal 'foo 42))
;;         (:form (tagged-literal 'foo 42))
;;         (get (tagged-literal 'foo 42) :nope :DEF)
;;         (tagged-literal? (tagged-literal 'foo 42))
;;         (tagged-literal? 42)
;;         (= (tagged-literal 'foo 42) (tagged-literal 'foo 42))
;;         (= (tagged-literal 'foo 42) (tagged-literal 'foo 43))
;;         (pr-str (tagged-literal 'my/tag [1 2 3]))
;;         (pr-str (tagged-literal 'foo nil))])
;;   => ["#foo 42" foo 42 :DEF true false true false "#my/tag [1 2 3]" "#foo nil"]
[(pr-str (tagged-literal 'foo 42))
 (:tag (tagged-literal 'foo 42))
 (:form (tagged-literal 'foo 42))
 (get (tagged-literal 'foo 42) :nope :DEF)
 (tagged-literal? (tagged-literal 'foo 42))
 (tagged-literal? 42)
 (= (tagged-literal 'foo 42) (tagged-literal 'foo 42))
 (= (tagged-literal 'foo 42) (tagged-literal 'foo 43))
 (pr-str (tagged-literal 'my/tag [1 2 3]))
 (pr-str (tagged-literal 'foo nil))]
;; expect: ["#foo 42" foo 42 :DEF true false true false "#my/tag [1 2 3]" "#foo nil"]
