;; M1 exit: a 100k-iteration loop*/recur runs with a constant Go stack —
;; recur is a LoopID-tagged signal caught by the owning loop* as a plain
;; Go loop (design/03 §5).
;; Oracle (Clojure 1.12, 2026-07-12): same form → 5000050000.
(loop* [i 0 acc 0]
  (if (< i 100001)
    (recur (+ i 1) (+ acc i))
    acc))
;; expect: 5000050000
