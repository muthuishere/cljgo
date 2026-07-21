;; clojure.walk/walk itself (fundamentals audit 2026-07): inner maps over
;; the elements, outer transforms the rebuilt collection; type is preserved
;; per branch (lists stay lists, vectors vectors, sets sets).
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (walk inc #(reduce + %) [1 2 3]) => 9
;;   (walk #(* 2 %) identity (list 1 2 3)) => (2 4 6)
;;   (walk inc set [1 2]) => #{2 3} — frozen via sorted print (sort (seq ...))
(require '[clojure.walk :as w])
[(w/walk inc (fn [c] (reduce + c)) [1 2 3])
 (w/walk (fn [x] (* 2 x)) identity (list 1 2 3))
 (sort (seq (w/walk inc identity #{1 2})))]
;; expect: [9 (2 4 6) (2 3)]
