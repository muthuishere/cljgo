;; some-> / some->>: thread while non-nil; first nil short-circuits to nil.
;; Oracle (clojure 1.12.5):
;; [(some-> {:a {:b 5}} :a :b inc) (some-> nil :a)
;;  (some-> {:a 1} :b inc) (some->> [1 2 3] (map inc) (reduce +))]
;; => [6 nil nil 9]
[(some-> {:a {:b 5}} :a :b inc)
 (some-> nil :a)
 (some-> {:a 1} :b inc)
 (some->> [1 2 3] (map inc) (reduce +))]
;; expect: [6 nil nil 9]
