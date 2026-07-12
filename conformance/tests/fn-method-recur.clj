;; Each fn* method is its own recur target: recur rebinds the params and
;; loops without re-dispatching arities (design/03 §5).
;; Oracle (Clojure 1.12, 2026-07-12): ((fn* [n acc] (if (< n 1) acc (recur (- n 1) (+ acc n)))) 10 0) → 55.
((fn* [n acc]
   (if (< n 1)
     acc
     (recur (- n 1) (+ acc n))))
 10 0)
;; expect: 55
