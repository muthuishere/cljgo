;; map-entry? (pkg/corelib/predicate_builtins.go): true only for a real map
;; entry — a plain 2-vector is NOT one (JVM: MapEntry is a distinct class).
;; oracle (clojure 1.12.5, 2026-07-21): (map-entry? (first {:a 1})) => true;
;; (map-entry? (first (sorted-map :a 1))) => true; [1 2] / {:a 1} / nil =>
;; false.
[(map-entry? (first {:a 1}))
 (map-entry? (first (sorted-map :a 1)))
 (map-entry? [1 2])
 (map-entry? {:a 1})
 (map-entry? nil)]
;; expect: [true true false false false]
