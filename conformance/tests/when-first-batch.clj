;; when-first (design/08 batch E, ADR 0022): [x coll] binds x to
;; (first coll), calling (seq coll) exactly once; nil on an empty/nil
;; coll; body has an implicit do.
;; oracle (clojure 1.12.5): confirmed interactively via `clojure -e`.
[(when-first [x [1 2 3]] (* x 10))
 (when-first [x []] :never)
 (when-first [x nil] :never)
 (let [counter (atom 0)]
   [(when-first [_ (range 5)]
      (swap! counter inc)
      (swap! counter inc)
      :bar)
    @counter])]
;; expect: [10 nil nil [:bar 2]]
