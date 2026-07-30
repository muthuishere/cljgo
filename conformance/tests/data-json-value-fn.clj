;; clojure.data.json read-str with :value-fn (ADR 0097): the value fn is called
;; as (value-fn key value) for each object entry and its result replaces the
;; value (key stays a string here, no :key-fn given).
;; oracle (org.clojure/data.json 2.5.1, verified 2026-07-27):
;;   (json/read-str "{\"a\":1,\"b\":2}" :value-fn (fn [k v] (* v 10)))
;;   => {"a" 10, "b" 20}
;; oracle: skip — external contrib dep, not on the default oracle classpath;
;; verified manually vs data.json 2.5.1.
(require '[clojure.data.json :as json])
(json/read-str "{\"a\":1,\"b\":2}" :value-fn (fn [k v] (* v 10)))
;; expect: {"a" 10, "b" 20}
