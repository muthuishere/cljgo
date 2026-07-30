;; clojure.data.json write-str :indent (ADR 0097): pretty-prints with 2-space
;; indentation, a newline after each '{'/'[' and entry, "key": value spacing,
;; and empty collections left compact — recursively, matching data.json.
;; oracle (org.clojure/data.json 2.5.1, verified 2026-07-27):
;;   (json/write-str (array-map :a 1 :b [1 2] :c {:d 3}) :indent true)
;;   => {\n  "a": 1,\n  "b": [\n    1,\n    2\n  ],\n  "c": {\n    "d": 3\n  }\n}
;; oracle: skip — external contrib dep, not on the default oracle classpath;
;; verified manually vs data.json 2.5.1.
(require '[clojure.data.json :as json])
(json/write-str (array-map :a 1 :b [1 2] :c {:d 3}) :indent true)
;; expect: "{\n  \"a\": 1,\n  \"b\": [\n    1,\n    2\n  ],\n  \"c\": {\n    \"d\": 3\n  }\n}"
