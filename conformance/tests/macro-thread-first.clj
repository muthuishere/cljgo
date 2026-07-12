;; M1 exit demo (design/00 §6): thread-first.
;; Oracle (clojure 1.12.5): (-> 5 (+ 3) (* 2)) => 16; bare-symbol form
;; (-> 10 (- 3) -) => (- (- 10 3)) => -7.
[(-> 5 (+ 3) (* 2)) (-> 10 (- 3) -)]
;; expect: [16 -7]
