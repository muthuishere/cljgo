;; every?/some/not-any?/not-every?/empty?/not-empty (clojure.core).
;; `some` here is the clojure.core seq predicate (NOT the Result track).
;; oracle (clojure 1.12.5):
;;   (every? even? [2 4 6]) => true
;;   (some even? [1 3 4]) => true
;;   (not-any? even? [1 3 5]) => true
;;   (not-every? even? [2 3]) => true
;;   (empty? []) => true ; (empty? [1]) => false
;;   (not-empty [1 2]) => [1 2] ; (not-empty []) => nil
[(every? even? [2 4 6])
 (some even? [1 3 4])
 (not-any? even? [1 3 5])
 (not-every? even? [2 3])
 (empty? [])
 (empty? [1])
 (not-empty [1 2])
 (not-empty [])]
;; expect: [true true true true true false [1 2] nil]
