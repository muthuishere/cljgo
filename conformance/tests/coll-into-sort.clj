;; into/sort/sort-by (clojure.core). Sets/maps use =/count to stay
;; ordering-agnostic; sorted seqs have a defined order.
;; oracle (clojure 1.12.5):
;;   (into [] (range 3)) => [0 1 2]
;;   (into {} [[:a 1] [:b 2]]) => {:a 1, :b 2}
;;   (count (into #{} [1 1 2 3])) => 3
;;   (sort [3 1 2]) => (1 2 3)
;;   (sort > [3 1 2]) => (3 2 1)
;;   (sort-by count ["ccc" "a" "bb"]) => ("a" "bb" "ccc")
[(into [] (range 3))
 (= {:a 1 :b 2} (into {} [[:a 1] [:b 2]]))
 (count (into #{} [1 1 2 3]))
 (sort [3 1 2])
 (sort > [3 1 2])
 (sort-by count ["ccc" "a" "bb"])]
;; expect: [[0 1 2] true 3 (1 2 3) (3 2 1) ("a" "bb" "ccc")]
