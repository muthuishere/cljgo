;; clojure.walk over sorted collections (core/walk.cljg): the coll? branch
;; rebuilds via (into (empty form) ...), so sortedness survives the walk.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (postwalk identity (sorted-map 2 :b 1 :a)) => {1 :a, 2 :b}
;;     (class => clojure.lang.PersistentTreeMap — still sorted)
;;   (postwalk #(if (number? %) (inc %) %) (sorted-set 3 1 2)) => #{2 3 4}
;;   (sorted? (postwalk identity (sorted-map 2 :b 1 :a))) => true
(require '[clojure.walk :as w])
[(w/postwalk identity (sorted-map 2 :b 1 :a))
 (w/postwalk (fn [x] (if (number? x) (inc x) x)) (sorted-set 3 1 2))
 (sorted? (w/postwalk identity (sorted-map 2 :b 1 :a)))]
;; expect: [{1 :a, 2 :b} #{2 3 4} true]
