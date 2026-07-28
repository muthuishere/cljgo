;; for — the :let / :when / :while modifiers (previously unsupported: cljgo
;; raised "macroexpanding let: Unsupported binding form: :when"). Modifiers
;; scope to the nearest preceding binding: :when skips the element and keeps
;; iterating, :while terminates THAT binding level's loop only (the enclosing
;; levels keep going), :let binds locals visible to later modifiers and to the
;; body. The result stays a lazy seq (an infinite source works).
;;
;; oracle (real `clojure` CLI, Clojure 1.12.5, run 2026-07-28):
;;   (for [n (range 1 11) :when (even? n)] n)              => (2 4 6 8 10)
;;   (for [n (range 10) :while (< n 4)] n)                 => (0 1 2 3)
;;   (for [x (range 3) :let [y (* x 10)]] y)               => (0 10 20)
;;   (for [x (range 4) y (range 4) :while (< y 2)] [x y])
;;     => ([0 0] [0 1] [1 0] [1 1] [2 0] [2 1] [3 0] [3 1])
;;   (for [x (range 4) :while (< x 2) y (range 2)] [x y])
;;     => ([0 0] [0 1] [1 0] [1 1])
;;   (for [x [1 2 3] y [1 2 3] :when (= x y)] [x y])       => ([1 1] [2 2] [3 3])
;;   (for [x (range 6) :when (odd? x) :let [y (* x x)] :while (< y 20)] y) => (1 9)
;;   (for [x (range 5) :let [y (- 4 x)] :while (> y 2)] [x y]) => ([0 4] [1 3])
;;   (for [x [] :when true] x)                             => ()
;;   (count (for [n (range 100000) :when (even? n)] n))    => 50000
;;   (take 3 (for [n (range) :when (even? n)] n))          => (0 2 4)
;;   (for [x (range 3) y (range 3) :when (> y x) :while (< x 2)] [x y])
;;     => ([0 1] [0 2] [1 2])
;;   (for [x (range 4) :when (even? x) :when (> x 0)] x)   => (2)
;;   (for [x (range 3) :let [a (* x 2) b (inc a)]] [a b])  => ([0 1] [2 3] [4 5])
;;   (for [x (range 3)] x)                                 => (0 1 2)
;;   (for [x (range 3) y (range 2)] [x y])
;;     => ([0 0] [0 1] [1 0] [1 1] [2 0] [2 1])
[(for [n (range 1 11) :when (even? n)] n)
 (for [n (range 10) :while (< n 4)] n)
 (for [x (range 3) :let [y (* x 10)]] y)
 (for [x (range 4) y (range 4) :while (< y 2)] [x y])
 (for [x (range 4) :while (< x 2) y (range 2)] [x y])
 (for [x [1 2 3] y [1 2 3] :when (= x y)] [x y])
 (for [x (range 6) :when (odd? x) :let [y (* x x)] :while (< y 20)] y)
 (for [x (range 5) :let [y (- 4 x)] :while (> y 2)] [x y])
 (for [x [] :when true] x)
 (count (for [n (range 100000) :when (even? n)] n))
 (take 3 (for [n (range) :when (even? n)] n))
 (for [x (range 3) y (range 3) :when (> y x) :while (< x 2)] [x y])
 (for [x (range 4) :when (even? x) :when (> x 0)] x)
 (for [x (range 3) :let [a (* x 2) b (inc a)]] [a b])
 (for [x (range 3)] x)
 (for [x (range 3) y (range 2)] [x y])]
;; expect: [(2 4 6 8 10) (0 1 2 3) (0 10 20) ([0 0] [0 1] [1 0] [1 1] [2 0] [2 1] [3 0] [3 1]) ([0 0] [0 1] [1 0] [1 1]) ([1 1] [2 2] [3 3]) (1 9) ([0 4] [1 3]) () 50000 (0 2 4) ([0 1] [0 2] [1 2]) (2) ([0 1] [2 3] [4 5]) (0 1 2) ([0 0] [0 1] [1 0] [1 1] [2 0] [2 1])]
