;; recur is legal in tail position through nested do and if forms.
;; Oracle (Clojure 1.12, 2026-07-12): same form → 6.
(loop* [n 3 acc 0]
  (if (< 0 n)
    (do 1 (if true (recur (- n 1) (+ acc n)) :nope))
    acc))
;; expect: 6
