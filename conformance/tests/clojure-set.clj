;; clojure.set (ADR 0022 batch/harness-misc): embedded core/set.cljg, a pure
;; port of clojure.set onto core.clj primitives.
;; oracle (clojure 1.12.5): every element verified with the `clojure` CLI:
;;   (union) => #{}; (intersection #{1 2 3} #{2 3 4} #{3 4 5}) => #{3};
;;   (difference #{1 2 3 4} #{1} #{2}) => #{4 3} — printed order matches
;;   cljgo's set printing for these small ints; membership equality is what
;;   we freeze (subset?/superset?/count) to stay order-independent.
(require '[clojure.set :as s :refer [subset?]])
[(s/union)
 (= (s/union #{1 2} #{2 3}) #{1 2 3})
 (= (s/intersection #{1 2 3} #{2 3 4} #{3 4 5}) #{3})
 (= (s/difference #{1 2 3 4} #{1} #{2}) #{3 4})
 (= (s/select even? #{1 2 3 4}) #{2 4})
 (= (s/project #{{:a 1 :b 2} {:a 3 :b 4}} [:a]) #{{:a 1} {:a 3}})
 (s/rename-keys {:a 1 :b 2} {:a :aa})
 (= (s/rename #{{:a 1 :b 2}} {:a :aa}) #{{:aa 1 :b 2}})
 (= (s/index #{{:a 1 :b 2}} [:a]) {{:a 1} #{{:a 1 :b 2}}})
 (s/map-invert {:a 1 :b 2})
 (= (s/join #{{:a 1 :b 2}} #{{:a 1 :c 3}}) #{{:a 1 :b 2 :c 3}})
 (subset? #{1 2} #{1 2 3})
 (subset? #{1 4} #{1 2 3})
 (s/superset? #{1 2 3} #{1 2})]
;; expect: [#{} true true true true true {:b 2, :aa 1} true true {1 :a, 2 :b} true true false true]
