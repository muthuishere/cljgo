;; Type ancestry for OUR types (ADR 0039): a defrecord/deftype's class
;; has REAL supers — the protocols it DECLARES (referenced by their
;; generated class name, e.g. user.P) plus the runtime interfaces its
;; instances genuinely implement (records: clojure.lang.Associative,
;; IPersistentMap, ... — pkg/lang's *Record really implements each, see
;; protocols_ancestry_test.go) plus Object. parents/ancestors/isa? see
;; them, and relationships derived from a SUPER flow into ancestors, all
;; exactly as clojure.core's bases/supers walks do. A protocol VALUE
;; itself follows the JVM's protocol-MAP reading: nil parents/ancestors/
;; descendants. extend-type does NOT alter ancestry (JVM: extend never
;; touches the class).
;;
;; harness: eval — class names (user.P, clojure.lang.Associative) resolve
;; at the interpreter's symbol-resolution level (ADR 0036/0039); AOT
;; emission of class-name symbols is deferred (suite runs interpreted,
;; ADR 0022 decision 4).
;; oracle: this EXACT file evaluated against clojure 1.12.5 (clojure CLI,
;; 2026-07-17) prints the same vector, modulo the one documented
;; divergence carried by class-refs-hierarchy.clj (none of the forms
;; below touch it).
(defprotocol P)
(defprotocol Q)
(defrecord R [] P)
(deftype T [] P)
(extend-type T Q)
[(contains? (parents R) user.P)
 (contains? (parents T) user.P)
 (contains? (ancestors R) user.P)
 (contains? (ancestors T) user.P)
 (contains? (ancestors R) clojure.lang.Associative)
 (contains? (parents R) clojure.lang.Associative)
 (contains? (ancestors R) Object)
 (contains? (ancestors T) Object)
 (contains? (ancestors T) clojure.lang.Associative)
 (ancestors P)
 (parents P)
 (descendants P)
 (isa? R user.P)
 (isa? T user.P)
 (isa? T Object)
 (isa? R user.Q)
 (contains? (ancestors T) user.Q)
 (let [h (derive (make-hierarchy) clojure.lang.Associative ::assoc-like)]
   (contains? (ancestors h R) ::assoc-like))
 (let [h (derive (make-hierarchy) R ::rec)]
   [(contains? (ancestors h R) ::rec)
    (contains? (ancestors h R) user.P)
    (isa? h R ::rec)])
 (->> (ancestors (-> (make-hierarchy)
                     (derive T ::datatype)
                     (derive ::datatype ::type))
                 T)
      (filter keyword?)
      (sort-by name)
      vec)]
;; expect: [true true true true true false true true false nil nil nil true true true false false true [true true true] [:user/datatype :user/type]]
