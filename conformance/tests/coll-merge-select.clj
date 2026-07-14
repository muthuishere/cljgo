;; merge/merge-with/select-keys/zipmap/frequencies/group-by (clojure.core).
;; Results compared with = (map key order is unspecified).
;; oracle (clojure 1.12.5):
;;   (merge {:a 1} {:b 2} {:a 3}) => {:a 3, :b 2}
;;   (merge-with + {:a 1 :b 2} {:a 10}) => {:a 11, :b 2}
;;   (select-keys {:a 1 :b 2 :c 3} [:a :c]) => {:a 1, :c 3}
;;   (zipmap [:a :b :c] [1 2 3]) => {:a 1, :b 2, :c 3}
;;   (frequencies [1 1 2 3 3 3]) => {1 2, 2 1, 3 3}
;;   (group-by even? (range 6)) => {true [0 2 4], false [1 3 5]}
[(= {:a 3 :b 2} (merge {:a 1} {:b 2} {:a 3}))
 (= {:a 11 :b 2} (merge-with + {:a 1 :b 2} {:a 10}))
 (= {:a 1 :c 3} (select-keys {:a 1 :b 2 :c 3} [:a :c]))
 (= {:a 1 :b 2 :c 3} (zipmap [:a :b :c] [1 2 3]))
 (= {1 2 2 1 3 3} (frequencies [1 1 2 3 3 3]))
 (= {true [0 2 4] false [1 3 5]} (group-by even? (range 6)))]
;; expect: [true true true true true true]
