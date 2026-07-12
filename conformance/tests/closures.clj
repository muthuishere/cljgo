;; fn* captures its lexical environment at evaluation time.
(def make-adder (fn* [n] (fn* [x] (+ x n))))
(def add5 (make-adder 5))
(add5 7)
;; expect: 12
