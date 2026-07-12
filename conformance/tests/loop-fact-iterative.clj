;; Iterative factorial via loop*/recur (design/03 §8 v1 exit). n=20 is
;; the largest int64-exact factorial; the constant-stack property at
;; n=100000 iterations is loop-constant-stack.clj.
;; Oracle (Clojure 1.12, 2026-07-12): same form → 2432902008176640000.
(loop* [n 20 acc 1]
  (if (< n 2)
    acc
    (recur (- n 1) (* acc n))))
;; expect: 2432902008176640000
