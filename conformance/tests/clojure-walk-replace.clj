;; clojure.walk/postwalk-replace + prewalk-replace (core/walk.cljg):
;; smap substitution over any data structure, leaves-first vs root-first
;; (postwalk-replace rewrites both the key and the value of {:a :a};
;; prewalk-replace replaces a matched subtree before descending into it).
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (postwalk-replace {:a 1} [:a :b {:a :a}]) => [1 :b {1 1}]
;;   (prewalk-replace {[1] [2]} [[1] {[1] :x}]) => [[2] {[2] :x}]
(require '[clojure.walk :as w])
[(w/postwalk-replace {:a 1} [:a :b {:a :a}])
 (w/prewalk-replace {[1] [2]} [[1] {[1] :x}])]
;; expect: [[1 :b {1 1}] [[2] {[2] :x}]]
