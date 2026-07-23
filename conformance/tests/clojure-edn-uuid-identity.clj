;; #uuid literals are `=` by value but never `identical?` across separate
;; reads, matching java.util.UUID's real object identity (ADR 0022 batch/
;; harness-misc, pkg/reader/tagged.go's *UUID pointer type — clojure-test-
;; suite edn_test/read_string.cljc "Tagged Elements UUIDs"). oracle
;; (clojure 1.12.5):
;;   (let [u1 #uuid "550e8400-e29b-41d4-a716-446655440000"
;;         u2 (clojure.edn/read-string "#uuid \"550e8400-e29b-41d4-a716-446655440000\"")]
;;     [(= u1 u2) (identical? u1 u2)]) => [true false]
(require '[clojure.edn :as edn])
(let [u1 #uuid "550e8400-e29b-41d4-a716-446655440000"
      u2 (edn/read-string "#uuid \"550e8400-e29b-41d4-a716-446655440000\"")]
  [(= u1 u2) (identical? u1 u2) (contains? #{u1} u2)])
;; expect: [true false true]
