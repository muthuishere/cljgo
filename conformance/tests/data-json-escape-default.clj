;; clojure.data.json write-str default escaping (ADR 0097): by default a
;; non-ASCII char is \uXXXX-escaped, '/' is escaped as '\/', and the control
;; escapes (\n \t \") are emitted — exactly data.json's defaults.
;; oracle (org.clojure/data.json 2.5.1, verified 2026-07-27):
;;   (json/write-str "héllo  / \n\t\"") => "héllo  \/ \n\t\""
;; oracle: skip — external contrib dep, not on the default oracle classpath;
;; verified manually vs data.json 2.5.1.
(require '[clojure.data.json :as json])
(json/write-str "héllo  / \n\t\"")
;; expect: "\"h\\u00e9llo  \\/ \\n\\t\\\"\""
