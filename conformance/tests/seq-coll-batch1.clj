;; Batch 1 seq/coll fns (ADR 0022, design/08 §5). Oracle (clojure 1.12):
;; (peek [1 2 3]) => 3; (pop [1 2 3]) => [1 2]; (last '(1 2 3)) => 3;
;; (butlast [1 2 3]) => (1 2); (find {:a 1} :a) => [:a 1]; (not= 1 2) => true.
[(last '(1 2 3)) (butlast [1 2 3]) (peek [1 2 3]) (pop [1 2 3])
 (peek '(1 2 3)) (pop '(1 2 3)) (subvec [1 2 3 4 5] 1 3) (rseq [1 2 3])
 (find {:a 1 :b 2} :b) (key (find {:a 1} :a)) (val (find {:a 1} :a))
 (set [1 2 2 3]) (disj #{1 2 3} 2) (empty [1 2]) (empty {:a 1})
 (drop-last [1 2 3]) (take-last 2 [1 2 3 4]) (ffirst [[1 2] [3 4]])
 (fnext [1 2 3]) (nfirst [[1 2 3]]) (not= 1 2) (not= 1 1)
 (compare 1 2) (compare 2 2) (sorted-set 3 1 2 1) (identical? :a :a)]
;; expect: [3 (1 2) 3 [1 2] 1 (2 3) [2 3] (3 2 1) [:b 2] :a 1 #{1 3 2} #{1 3} [] {} (1 2) (3 4) 1 2 (2 3) true false -1 0 #{1 2 3} true]
