;; Batch A3: iteration (1.11) — the paginated-API driver. (step k)
;; produces a ret; while (somef ret) the iteration emits (vf ret) and,
;; when (kf ret) is non-nil, continues with (step (kf ret)). Both
;; consumption paths are covered: seq and reduce (incl. early
;; termination via reduced), plus the default somef/vf/kf and an
;; explicit :somef/:initk.
;; Oracle (clojure 1.12.5): verified 2026-07-23.
(def pages {0 {:items [1 2] :next 1}
            1 {:items [3 4] :next 2}
            2 {:items [5] :next nil}})
(defn fetch [k] (get pages (or k 0)))
(def it (iteration fetch :kf :next :vf :items))
(def it2 (iteration (fn [k] (when (< (or k 0) 3) (inc (or k 0))))))
[(seq it)
 (reduce (fn [acc v] (into acc v)) [] it)
 (vec (iteration fetch :kf :next :vf :items :somef some? :initk nil))
 (seq it2)
 (reduce + 0 it2)
 (reduce (fn [acc v] (if (>= acc 3) (reduced :done) (+ acc (count v)))) 0 it)]
;; expect: [([1 2] [3 4] [5]) [1 2 3 4 5] [[1 2] [3 4] [5]] (1 2 3) 6 :done]
