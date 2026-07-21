;; map-entry? (fundamentals audit 2026-07, clojure.walk substrate): true
;; for real map entries (what map seqs yield), false for plain 2-vectors.
;; Entries still behave as vectors (vector?, destructuring, key/val).
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (map-entry? (first {:a 1})) => true
;;   (map-entry? [1 2]) => false
;;   (map-entry? 1) => false
;;   (vector? (first {:a 1})) => true
;;   (key (first {:a 1})) => :a ; (val (first {:a 1})) => 1
[(map-entry? (first {:a 1}))
 (map-entry? [1 2])
 (map-entry? 1)
 (vector? (first {:a 1}))
 (key (first {:a 1}))
 (val (first {:a 1}))]
;; expect: [true false false true :a 1]
