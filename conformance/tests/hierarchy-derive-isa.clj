;; Hierarchies (ADR 0022 Track E, design/08 batch E): make-hierarchy/
;; derive/isa?/parents/ancestors/descendants/underive, both the global
;; (2-arg) form and an explicit hierarchy value (3-arg form, no namespace
;; requirement). Namespaced keywords used throughout are unique to this
;; file to avoid cross-file interference via the process-global hierarchy.
;; Set-valued results are compared via `=` (not printed) since
;; PersistentHashSet iteration order is not semantically meaningful.
;; oracle (clojure 1.12.5): confirmed interactively via `clojure -e`.
[(let [h0 (make-hierarchy)]
   (derive ::hd-c1 ::hd-p1)
   (derive ::hd-c1 ::hd-p2)
   (derive ::hd-p1 ::hd-gp)
   [(isa? :a :a)
    (isa? ::hd-c1 ::hd-p1)
    (isa? ::hd-c1 ::hd-gp)
    (isa? ::hd-p2 ::hd-gp)
    (= #{::hd-p1 ::hd-p2} (parents ::hd-c1))
    (= #{::hd-p1 ::hd-p2 ::hd-gp} (ancestors ::hd-c1))
    (= #{::hd-c1 ::hd-p1} (descendants ::hd-gp))
    (ancestors :cljgo-test-hierarchy/unknown-tag)
    (isa? [::hd-c1 ::hd-c1] [::hd-c1 ::hd-c1])
    (do (underive ::hd-c1 ::hd-p1) (isa? ::hd-c1 ::hd-p1))
    (isa? (derive h0 :x :y) :x :y)
    h0])]
;; expect: [[true true true false true true true nil true false true {:parents {}, :descendants {}, :ancestors {}}]]
