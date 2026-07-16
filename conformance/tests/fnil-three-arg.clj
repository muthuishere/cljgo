;; fnil (batch/error-files): the 3-arg default form ([f a b c]) was missing
;; entirely — (fnil f 100 200 300) applied to 3+ args threw "wrong number
;; of args" since only the 1- and 2-default arities existed.
;; oracle (clojure 1.12.5): (let [f (fnil vector 100 200 300)]
;; [(f nil nil nil) (f 1 nil nil) (f nil nil nil :extra)]) =>
;; [[100 200 300] [1 200 300] [100 200 300 :extra]]
(let [f (fnil vector 100 200 300)]
  [(f nil nil nil) (f 1 nil nil) (f nil nil nil :extra)])
;; expect: [[100 200 300] [1 200 300] [100 200 300 :extra]]
