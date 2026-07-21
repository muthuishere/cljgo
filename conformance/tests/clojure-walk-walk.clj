;; clojure.walk/walk (core/walk.cljg, fundamentals audit 2026-07): inner over
;; each element, outer over the rebuilt same-type result.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (walk inc identity '(1 2 3)) => (2 3 4)
;;   (walk inc (fn [c] (reduce + c)) [1 2 3]) => 9
;;   (walk identity identity {:a 1}) => {:a 1}
(require '[clojure.walk :as w])
[(w/walk inc identity (list 1 2 3))
 (w/walk inc (fn [c] (reduce + c)) [1 2 3])
 (w/walk identity identity {:a 1})]
;; expect: [(2 3 4) 9 {:a 1}]
