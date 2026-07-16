;; get-in (3-arity), interleave, and partition (batch/error-files): three
;; unrelated but similarly-shaped bugs fixed together —
;;  - get-in's 3-arity used `contains?` to detect a missing key, which
;;    THROWS on a non-associative intermediate value (a keyword mid-path);
;;    real Clojure uses `get` with a private sentinel + `identical?` instead,
;;    so a non-associative miss just yields not-found.
;;  - interleave/partition were fixed at their 2-arg core, missing the
;;    variadic (interleave c1 c2 & colls) and (partition n step [pad] coll)
;;    arities entirely (a compile-time "wrong number of args" panic).
;; oracle (clojure 1.12.5):
;;   (get-in {:a {:b {:c :d}}} [:a :b :c :d] :not-found) => :not-found
;;   (apply interleave [[1 2 3 4 5] ["a" "b" "c"] "12"]) => (1 "a" \1 2 "b" \2)
;;   (partition 3 1 [:a :a :a] nil) => ()
;;   (partition 4 4 [:a] (range 10)) => ((0 1 2 3) (4 5 6 7) (8 9 :a))
[[(get-in {:a {:b {:c :d}}} [:a :b :c :d] :not-found)
  (get-in {:a 1} [:a :b] :not-found)
  (get-in {:a {:b 5}} [:a :b])]
 (apply interleave [[1 2 3 4 5] ["a" "b" "c"] "12"])
 [(partition 3 1 [:a :a :a] nil)
  (partition 4 4 [:a] (range 10))
  (partition 3 (range 7))]]
;; expect: [[:not-found :not-found 5] (1 "a" \1 2 "b" \2) [() ((0 1 2 3) (4 5 6 7) (8 9 :a)) ((0 1 2) (3 4 5))]]
