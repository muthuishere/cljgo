;; Regression: stale hash cache on array-map assoc (PR #30 flagged it;
;; fixed in pkg/lang/persistentarraymap.go Map.clone — see PROVENANCE.md
;; "Stale hash cache on array-map assoc"). conj/assoc/merge onto a map
;; fetched out of an existing set (its hash already cached by membership)
;; must produce a value that hashes like a fresh equal map, so sets/maps
;; built from it stay `=`-addressable; the source collection stays intact.
;; oracle (clojure 1.12.5 CLI): every element below verified to print true.
(require '[clojure.set :as s])
(let [xrel #{{:a 1 :b 2}}
      yrel #{{:a 1 :c 3}}
      joined (s/join xrel yrel)
      src #{{:a 1} {:a 2}}
      member (first (filter #(= % {:a 1}) src))
      grown (conj member [:b 9])
      out #{grown}
      keyed {(assoc member :k 1) :hit}
      v [{:x 1}]
      velt (conj (v 0) [:y 2])]
  [(= joined #{{:a 1 :b 2 :c 3}})
   (contains? joined {:a 1 :b 2 :c 3})
   (= xrel #{{:a 1 :b 2}})
   (= yrel #{{:a 1 :c 3}})
   (= grown {:a 1 :b 9})
   (contains? out {:a 1 :b 9})
   (= src #{{:a 1} {:a 2}})
   (= :hit (get keyed {:a 1 :k 1}))
   (= (merge member {:m 5}) {:a 1 :m 5})
   (contains? #{(merge member {:m 5})} {:a 1 :m 5})
   (= velt {:x 1 :y 2})
   (= v [{:x 1}])])
;; expect: [true true true true true true true true true true true true]
