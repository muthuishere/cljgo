;; cast / bases / supers over cljgo's real type ancestry (ADR 0039; tail
;; wave, 2026-07-23). cast rides the SAME designator matching as
;; instance? (classNameMatchesValue), so the two can never disagree; its
;; ClassCastException message is byte-shaped like the JVM's Class.cast.
;; bases/supers ride -type-bases/-type-supers: deftype/defrecord markers
;; report their protocols + genuinely implemented interfaces + Object.
;; DEVIATIONS (documented, not frozen): no JVM class hierarchy is
;; fabricated (ADR 0036), so (supers Long) is #{java.lang.Object} here
;; where the JVM adds Number/Comparable/Constable/ConstantDesc, and
;; (cast Number 5) throws here (instance?-consistent) where the JVM
;; returns 5.
;; oracle (clojure 1.12.5, 2026-07-23, scratch tailwave/o1.clj+o3.clj):
;;   (cast Long 5) => 5; (cast Long nil) => nil
;;   (cast String 5) throws ClassCastException
;;     "Cannot cast java.lang.Long to java.lang.String"
;;   (cast String 5) caught by (catch ClassCastException e ...) — the
;;     real exception type, per the #99 typed-exception ancestry
;;   (deftype QQ []) (sort (map str (bases QQ)))
;;     => ("clojure.lang.IType" "java.lang.Object")
;;   (defrecord RR []) (contains? (supers RR) clojure.lang.IRecord) => true
;;   (bases Object) => nil; (supers Object) => nil
(deftype QQ [])
(defrecord RR [])
[(cast Long 5)
 (cast Long nil)
 (try (cast String 5) (catch ClassCastException e (ex-message e)))
 (vec (sort (map str (bases QQ))))
 (contains? (supers RR) clojure.lang.IRecord)
 (bases Object)
 (supers Object)]
;; expect: [5 nil "Cannot cast java.lang.Long to java.lang.String" ["clojure.lang.IType" "java.lang.Object"] true nil nil]
