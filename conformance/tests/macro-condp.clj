;; condp: dispatch by a binary predicate; ternary `t :>> f` calls (f (pred t expr)).
;; Oracle (clojure 1.12.5):
;; [(condp = 3 1 :a 3 :c :none) (condp = 5 1 :a 3 :c :none)
;;  (condp = 2 1 :a 2 :>> (fn [x] [:got x]) :none) (condp > 5 10 :big 1 :small)]
;; => [:c :none [:got true] :big]
[(condp = 3 1 :a 3 :c :none)
 (condp = 5 1 :a 3 :c :none)
 (condp = 2 1 :a 2 :>> (fn [x] [:got x]) :none)
 (condp > 5 10 :big 1 :small)]
;; expect: [:c :none [:got true] :big]
