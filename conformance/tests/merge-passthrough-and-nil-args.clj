;; merge (batch/error-files): real Clojure's `merge` is (reduce1 #(conj (or
;; %1 {}) %2) maps) guarded by (some identity maps) — a single non-map arg
;; passes through unchanged (reduce1 with one element just returns it, `%1`
;; is never even computed), nil args are skipped via `(or %1 {})`, but a
;; genuinely bad non-map arg alongside another still blows up in `conj`
;; (real Clojure: conj-ing a non-map/non-MapEntry item onto a map throws).
;; oracle (clojure 1.12.5): [(merge :foo) (merge nil {:a 1}) (merge {:a 1}
;; nil {:b 2})] => [:foo {:a 1} {:a 1, :b 2}]; (merge nil (range)) throws
;; (an infinite seq is neither a map nor a MapEntry).
[[(merge :foo) (merge nil {:a 1}) (merge {:a 1} nil {:b 2})]
 (try (merge nil (range)) :nothrow (catch Exception _e :threw))]
;; expect: [[:foo {:a 1} {:a 1, :b 2}] :threw]
