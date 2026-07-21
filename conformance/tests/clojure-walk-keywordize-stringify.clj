;; clojure.walk/keywordize-keys + stringify-keys (core/walk.cljg):
;; recursive map-key transformation; non-matching keys pass through.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (keywordize-keys {"a" 1 "b" {"c" 2} :d 3}) => {:a 1, :b {:c 2}, :d 3}
;;   (stringify-keys {:a 1 :b {:c 2} "d" 3}) => {"a" 1, "b" {"c" 2}, "d" 3}
;;   (keywordize-keys nil) => nil
(require '[clojure.walk :as w])
[(w/keywordize-keys {"a" 1 "b" {"c" 2} :d 3})
 (w/stringify-keys {:a 1 :b {:c 2} "d" 3})
 (w/keywordize-keys nil)]
;; expect: [{:a 1, :b {:c 2}, :d 3} {"a" 1, "b" {"c" 2}, "d" 3} nil]
