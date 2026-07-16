;; The global (2-arg) `derive` requires BOTH tag and parent to be
;; namespace-qualified — a bare keyword throws (the 3-arg explicit-
;; hierarchy form has no such requirement, see hierarchy-derive-isa.clj).
;; oracle (clojure 1.12.5): (derive :a :b) throws AssertionError,
;; message "Assert failed: (namespace parent)".
(try (derive :a :b) :nothrow
     (catch Exception e (ex-message e)))
;; expect: "Assert failed: (namespace parent)"
