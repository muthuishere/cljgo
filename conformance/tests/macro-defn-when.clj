;; M1 exit demo (design/00 §6): defn + when from core.clj.
;; Oracle (clojure 1.12.5): (f 2) => 4, (f 7) => nil.
(defn f [x] (when (< x 5) (* x 2)))
[(f 2) (f 7)]
;; expect: [4 nil]
