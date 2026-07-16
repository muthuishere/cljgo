;; atom :validator/:meta options + get-validator/set-validator! (design/08
;; batch E, ADR 0022): a validator that rejects the initial value throws
;; at construction; swap!/reset! run through it too, on failure leaving
;; the atom unchanged; set-validator! on an already-invalid state throws.
;; oracle (clojure 1.12.5): confirmed interactively via `clojure -e`.
[(let [a (atom 0 :validator even? :meta {:foo "foo"})]
   [(= even? (get-validator a)) (meta a) (deref a)])
 (try (atom 1 :validator even?) :nothrow (catch Exception _e :threw))
 (let [a (atom 0 :validator even?)]
   [(try (swap! a inc) :nothrow (catch Exception _e :threw))
    (swap! a + 2)
    (try (reset! a 3) :nothrow (catch Exception _e :threw))
    (reset! a 10)])
 (try (set-validator! (atom 1) even?) :nothrow (catch Exception _e :threw))]
;; expect: [[true {:foo "foo"} 0] :threw [:threw 2 :threw 10] :threw]
