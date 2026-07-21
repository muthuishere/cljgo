;; clojure.walk replace + key-transform fns (fundamentals audit 2026-07):
;; postwalk-replace/prewalk-replace substitute via an smap anywhere in a
;; structure; keywordize-keys/stringify-keys recursively convert map keys.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (postwalk-replace {:a 1 :b 2} [:a :b :c [:a]]) => [1 2 :c [1]]
;;   (prewalk-replace {:a 1} {:a :a}) => {1 1}
;;   (keywordize-keys {"a" 1 "b" {"c" 2} :d 3}) => {:a 1, :b {:c 2}, :d 3}
;;   (keywordize-keys [{"a" 1} {"b" 2}]) => [{:a 1} {:b 2}]
;;   (stringify-keys {:a 1 :b {:c 2} "d" 3}) => {"a" 1, "b" {"c" 2}, "d" 3}
(require '[clojure.walk :as w])
[(w/postwalk-replace {:a 1 :b 2} [:a :b :c [:a]])
 (w/prewalk-replace {:a 1} {:a :a})
 (w/keywordize-keys {"a" 1 "b" {"c" 2} :d 3})
 (w/keywordize-keys [{"a" 1} {"b" 2}])
 (w/stringify-keys {:a 1 :b {:c 2} "d" 3})]
;; expect: [[1 2 :c [1]] {1 1} {:a 1, :b {:c 2}, :d 3} [{:a 1} {:b 2}] {"a" 1, "b" {"c" 2}, "d" 3}]
