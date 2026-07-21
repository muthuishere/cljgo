;; clojure.walk/postwalk + prewalk (core/walk.cljg): depth-first post-/
;; pre-order traversal across nested maps, vectors, sets and lists.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (postwalk #(if (number? %) (inc %) %) {:a 1 :b [1 2 #{3}] :c (list 4 5)})
;;     => {:a 2, :b [2 3 #{4}], :c (5 6)}
;;   (prewalk #(if (number? %) (inc %) %) [1 [2 [3]]]) => [2 [3 [4]]]
(require '[clojure.walk :as w])
[(w/postwalk (fn [x] (if (number? x) (inc x) x)) {:a 1 :b [1 2 #{3}] :c (list 4 5)})
 (w/prewalk (fn [x] (if (number? x) (inc x) x)) [1 [2 [3]]])]
;; expect: [{:a 2, :b [2 3 #{4}], :c (5 6)} [2 [3 [4]]]]
