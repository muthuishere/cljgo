;; letfn (fundamentals batch 1): local fns that see each other — mutual
;; recursion, multi-arity, self-recursion, shadowing an outer local,
;; destructured and variadic params, empty binding vector.
;; oracle (clojure 1.12.5), each element:
;;   (letfn [(f [x] (g x)) (g [x] (* 2 x))] (f 21))                => 42
;;   (letfn [(f ([x] (f x 10)) ([x y] (+ x y)))] (f 5))            => 15
;;   (letfn [] 7)                                                  => 7
;;   (letfn [(fact [n] (if (zero? n) 1 (* n (fact (dec n)))))]
;;     (fact 5))                                                   => 120
;;   (let [f :outer] (letfn [(f [] :inner)] (f)))                  => :inner
;;   (letfn [(f [[a b]] (+ a b))] (f [3 4]))                       => 7
;;   (letfn [(f [& xs] (count xs))] (f 1 2 3))                     => 3
[(letfn [(f [x] (g x)) (g [x] (* 2 x))] (f 21))
 (letfn [(f ([x] (f x 10)) ([x y] (+ x y)))] (f 5))
 (letfn [] 7)
 (letfn [(fact [n] (if (zero? n) 1 (* n (fact (dec n)))))] (fact 5))
 (let [f :outer] (letfn [(f [] :inner)] (f)))
 (letfn [(f [[a b]] (+ a b))] (f [3 4]))
 (letfn [(f [& xs] (count xs))] (f 1 2 3))]
;; expect: [42 15 7 120 :inner 7 3]
