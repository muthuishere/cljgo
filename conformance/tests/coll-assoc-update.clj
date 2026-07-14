;; assoc/update/dissoc/get-in/assoc-in/update-in (clojure.core). Map results
;; compared with = (order-independent) to stay ordering-agnostic.
;; oracle (clojure 1.12.5):
;;   (assoc {:a 1} :b 2) => {:a 1, :b 2}
;;   (update {:a 1} :a inc) => {:a 2}
;;   (get-in {:a {:b 5}} [:a :b]) => 5
;;   (get-in {:a {:b 5}} [:a :c] :missing) => :missing
;;   (assoc-in {:a {:b 1}} [:a :c] 9) => {:a {:b 1, :c 9}}
;;   (update-in {:a {:b 1}} [:a :b] inc) => {:a {:b 2}}
;;   (dissoc {:a 1 :b 2} :a) => {:b 2}
[(= {:a 1 :b 2} (assoc {:a 1} :b 2))
 (= {:a 2} (update {:a 1} :a inc))
 (get-in {:a {:b 5}} [:a :b])
 (get-in {:a {:b 5}} [:a :c] :missing)
 (= {:a {:b 1 :c 9}} (assoc-in {:a {:b 1}} [:a :c] 9))
 (= {:a {:b 2}} (update-in {:a {:b 1}} [:a :b] inc))
 (= {:b 2} (dissoc {:a 1 :b 2} :a))]
;; expect: [true true 5 :missing true true true]
