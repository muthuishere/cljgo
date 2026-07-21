;; distinct? (fundamentals audit 2026-07): true when no two args are =.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (distinct? 1) => true
;;   (distinct? 1 2) => true
;;   (distinct? 1 1) => false
;;   (distinct? 1 2 3 1) => false
;;   (distinct? :a :b :c) => true
;;   (apply distinct? [1 2 3 4 2]) => false
[(distinct? 1)
 (distinct? 1 2)
 (distinct? 1 1)
 (distinct? 1 2 3 1)
 (distinct? :a :b :c)
 (apply distinct? [1 2 3 4 2])]
;; expect: [true true false false true false]
