;; record? (pkg/corelib/predicate_builtins.go): true only for a defrecord
;; instance (JVM: instance? IRecord) — a plain map is map? but never
;; record?.
;; oracle (clojure 1.12.5, 2026-07-21): with (defrecord Q [a]):
;; (record? (->Q 1)) => true; (record? {:a 1}) / (record? nil) => false;
;; (map? (->Q 1)) => true.
(defrecord Q [a])
[(record? (->Q 1))
 (record? {:a 1})
 (record? nil)
 (map? (->Q 1))]
;; expect: [true false false true]
