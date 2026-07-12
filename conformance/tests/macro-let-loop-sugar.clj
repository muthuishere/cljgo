;; let/loop (no star) are core macros over let*/loop* — destructuring
;; deferred, simple symbols only (design/03 §5, M1 waiver).
(let [x 3 y 4]
  (loop [i 0 acc 0]
    (if (< i x) (recur (+ i 1) (+ acc y)) acc)))
;; expect: 12
