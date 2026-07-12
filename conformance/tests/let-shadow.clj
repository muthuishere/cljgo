;; Sequential let* bindings shadow left-to-right; a closure made between
;; two bindings of the same name keeps seeing the earlier frame.
(let* [x 1
       f (fn* [] x)
       x (+ x 10)]
  [(f) x])
;; expect: [1 11]
