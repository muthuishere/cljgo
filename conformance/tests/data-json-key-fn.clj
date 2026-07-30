;; clojure.data.json read-str with :key-fn (ADR 0097): the key fn is applied to
;; every object key string, at every nesting depth, before it becomes a map
;; key — so (:key-fn keyword) yields keyword-keyed maps throughout.
;; oracle (org.clojure/data.json 2.5.1, verified 2026-07-27):
;;   (json/read-str "{\"a\":1,\"nested\":{\"b\":2}}" :key-fn keyword)
;;   => {:a 1, :nested {:b 2}}
;; oracle: skip — external contrib dep, not on the default oracle classpath;
;; verified manually vs data.json 2.5.1.
(require '[clojure.data.json :as json])
(json/read-str "{\"a\":1,\"nested\":{\"b\":2}}" :key-fn keyword)
;; expect: {:a 1, :nested {:b 2}}
