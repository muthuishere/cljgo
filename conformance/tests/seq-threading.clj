;; The canonical ->> pipeline over the seq library (clojure.core).
;; oracle (clojure 1.12.5):
;;   (->> (range 10) (filter odd?) (map (fn [x] (* x x))) (reduce +)) => 165
(->> (range 10) (filter odd?) (map (fn [x] (* x x))) (reduce +))
;; expect: 165
