;; contains? on a string checks index bounds (not value membership, and not
;; an error) — a non-integer key still throws, since there's nothing to
;; cast. contains? also works on transients (probed via ValAtDefault/
;; ITransientSet.Contains, since transients have no ContainsKey).
;; oracle (clojure 1.12.5): [(contains? "abc" 2) (contains? "abc" 3)
;; (contains? "abc" -1)] => [true false false]; (contains? "abc" "a") throws;
;; (contains? (transient {:x 1}) :x) => true; (contains? (transient [0 1]) 5)
;; => false; (contains? (transient #{1 2}) 2) => true.
[[(contains? "abc" 2) (contains? "abc" 3) (contains? "abc" -1)]
 (try (contains? "abc" "a") :nothrow (catch Exception _e :threw))
 [(contains? (transient {:x 1}) :x)
  (contains? (transient [0 1]) 5)
  (contains? (transient #{1 2}) 2)]]
;; expect: [[true false false] :threw [true false true]]
