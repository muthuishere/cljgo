;; clojure.data.json write-str (ADR 0097): a map becomes a JSON object with
;; keyword keys rendered as their name, a vector becomes an array, and scalars
;; render naturally. The result String prints identically in REPL and binary.
;; oracle (org.clojure/data.json 2.5.1, verified 2026-07-27):
;;   (json/write-str {:a 1 :b [2 3.5 true nil]}) => {"a":1,"b":[2,3.5,true,null]}
;; oracle: skip — external contrib dep, not on the default oracle classpath;
;; verified manually vs data.json 2.5.1.
(require '[clojure.data.json :as json])
(json/write-str {:a 1 :b [2 3.5 true nil]})
;; expect: "{\"a\":1,\"b\":[2,3.5,true,null]}"
