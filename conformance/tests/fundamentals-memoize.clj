;; memoize (fundamentals audit 2026-07): caches by full argument list;
;; repeat = calls return the cached value without re-invoking f.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   with (def calls (atom 0)), (def mf (memoize (fn [x] (swap! calls inc) (* x 10)))):
;;   [(mf 2) (mf 2) (mf 3) @calls] => [20 20 30 2]
;;   variadic keys: (def mf2 (memoize (fn [& xs] (apply + xs))))
;;   [(mf2 1 2) (mf2) (mf2 1 2 3)] => [3 0 6]
(def calls (atom 0))
(def mf (memoize (fn [x] (swap! calls inc) (* x 10))))
(def mf2 (memoize (fn [& xs] (apply + xs))))
[(mf 2) (mf 2) (mf 3) (deref calls) (mf2 1 2) (mf2) (mf2 1 2 3)]
;; expect: [20 20 30 2 3 0 6]
