;; recur outside tail position is an analysis-time error.
;; Oracle (Clojure 1.12, 2026-07-12): "Can only recur from tail position".
(loop* [x 1] (+ (recur 2) 1))
;; expect-error: recur from tail position
