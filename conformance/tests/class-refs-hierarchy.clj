;; Class refs (ADR 0036): well-known JVM class names (String, Object,
;; java.lang.String, ...) resolve — only after normal var resolution
;; fails — to interned, opaque ClassRef values with identity equality.
;; They are valid hierarchy TAGS (derive/isa?/underive), `class?` is true
;; for them (and for deftype/defrecord types), and `descendants` throws
;; on classes exactly as the JVM does. NO inheritance is fabricated:
;; (parents String) is nil unless something was explicitly derived.
;;
;; harness: eval — class refs resolve at the interpreter's symbol-
;; resolution level (ADR 0036); AOT emission of bare class-name symbols
;; is deferred (the suite runs interpreted, ADR 0022 decision 4).
;; oracle: skip — deliberate documented divergence: on the JVM
;; (parents String) reports type-inheritance ancestry (Object, Comparable,
;; ...) which cljgo will not fake. Every class-semantics fact below WAS
;; oracle-verified per-form against clojure 1.12.5 (ADR 0036 evidence,
;; 2026-07-16): derive-class-tag ok, class-as-global-parent throws,
;; (descendants Object) / (descendants h Object) / (descendants SomeRecord)
;; all throw "Can't get descendants of classes", (parents Object) => nil,
;; (class? String) => true.
(defrecord CrRec [])
[(pr-str String)
 (class? String)
 (class? CrRec)
 (class? 42)
 (= String java.lang.String)
 (do (derive String ::cr-object) (isa? String ::cr-object))
 (= #{::cr-object} (parents String))
 (do (underive String ::cr-object) (parents String))
 (parents Object)
 (try (derive ::cr-tag String) :nothrow (catch Exception e :threw))
 (try (descendants Object) :nothrow (catch Exception e (ex-message e)))
 (try (descendants (make-hierarchy) Object) :nothrow (catch Exception e :threw))
 (try (descendants CrRec) :nothrow (catch Exception e :threw))
 (do (derive CrRec ::cr-rec)
     (let [ds (descendants ::cr-rec)]
       (underive CrRec ::cr-rec)
       (= #{CrRec} ds)))
 (= (derive (make-hierarchy) String ::cr-object)
    {:parents {String #{::cr-object}}
     :ancestors {String #{::cr-object}}
     :descendants {::cr-object #{String}}})
 (instance? (identity String) "x")
 (instance? (identity Object) nil)]
;; expect: ["java.lang.String" true true false true true true nil nil :threw "Can't get descendants of classes" :threw :threw true true true false]
