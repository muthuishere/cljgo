;; reduce (2- and 3-arg) and reduce-kv (clojure.core).
;; oracle (clojure 1.12.5):
;;   (reduce + 0 (range 1 11)) => 55
;;   (reduce + (range 1 11)) => 55
;;   (reduce-kv (fn [a k v] (+ a v)) 0 {:a 1 :b 2 :c 3}) => 6
[(reduce + 0 (range 1 11))
 (reduce + (range 1 11))
 (reduce-kv (fn [a k v] (+ a v)) 0 {:a 1 :b 2 :c 3})]
;; expect: [55 55 6]
