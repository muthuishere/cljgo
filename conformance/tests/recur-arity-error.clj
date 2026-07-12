;; recur arg count must match the enclosing loop*'s binding count,
;; checked at analysis time.
;; Oracle (Clojure 1.12, 2026-07-12): "Mismatched argument count to recur, expected: 2 args, got: 1".
(loop* [x 1 y 2] (recur 5))
;; expect-error: argument count to recur, expected: 2 args, got: 1
