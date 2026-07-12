;; Multi-arity fn*: exact fixed arity wins, else the variadic method;
;; the rest param binds a seq of the extra args.
(def f (fn* ([] :zero)
            ([x] x)
            ([x & xs] [x xs])))
[(f) (f 1) (f 1 2 3)]
;; expect: [:zero 1 [1 (2 3)]]
