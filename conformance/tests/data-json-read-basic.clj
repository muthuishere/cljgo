;; clojure.data.json read-str (ADR 0097): a JSON object parses to a map with
;; STRING keys by default, arrays to vectors, and scalars to natural cljgo
;; values (long, double, true/false/nil). The Go host codec (pkg/bri/cljson)
;; backs it; this asserts the parsed shape prints identically in REPL and
;; compiled binary.
;; oracle (org.clojure/data.json 2.5.1, verified 2026-07-27):
;;   (json/read-str "{\"a\":1,\"b\":[2,3.5,true,null]}")
;;   => {"a" 1, "b" [2 3.5 true nil]}
;; oracle: skip — org.clojure/data.json is an external contrib dep, not on the
;; default `clojure` oracle classpath; verified manually vs data.json 2.5.1.
(require '[clojure.data.json :as json])
(json/read-str "{\"a\":1,\"b\":[2,3.5,true,null]}")
;; expect: {"a" 1, "b" [2 3.5 true nil]}
