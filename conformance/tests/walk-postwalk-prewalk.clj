;; clojure.walk postwalk/prewalk (fundamentals audit 2026-07): depth-first
;; transforms; postwalk rewrites leaves first, prewalk the root first.
;; List-ness survives a walk. Visit ORDER is frozen via atom traces —
;; entries appear as [k v] (a real map entry: JVM MapEntry, cljgo -map-entry).
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (postwalk #(if (number? %) (inc %) %) {:a 1 :b [2 3]}) => {:a 2, :b [3 4]}
;;   (prewalk #(if (vector? %) (seq %) %) [1 [2 [3]]]) => (1 (2 (3)))
;;   postwalk trace on {:a [1 2]} => [:a 1 2 [1 2] [:a [1 2]] {:a [1 2]}]
;;   prewalk trace on {:a [1 2]} => [{:a [1 2]} [:a [1 2]] :a [1 2] 1 2]
;;   (postwalk identity '(1 (2) 3)) => (1 (2) 3), still list?
(require '[clojure.walk :as w])
(def post-trace (atom []))
(w/postwalk (fn [x] (swap! post-trace conj x) x) {:a [1 2]})
(def pre-trace (atom []))
(w/prewalk (fn [x] (swap! pre-trace conj x) x) {:a [1 2]})
[(w/postwalk (fn [x] (if (number? x) (inc x) x)) {:a 1 :b [2 3]})
 (w/prewalk (fn [x] (if (vector? x) (seq x) x)) [1 [2 [3]]])
 (deref post-trace)
 (deref pre-trace)
 (w/postwalk identity '(1 (2) 3))
 (list? (w/postwalk identity '(1 2)))]
;; expect: [{:a 2, :b [3 4]} (1 (2 (3))) [:a 1 2 [1 2] [:a [1 2]] {:a [1 2]}] [{:a [1 2]} [:a [1 2]] :a [1 2] 1 2] (1 (2) 3) true]
