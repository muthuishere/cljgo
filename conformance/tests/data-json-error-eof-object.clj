;; clojure.data.json read-str on a truncated object throws an EOF-in-object
;; message (ADR 0097). data.json raises "JSON error (EOF in object)".
;; oracle (org.clojure/data.json 2.5.1, verified 2026-07-27):
;;   (json/read-str "{\"a\":1") => throws "JSON error (EOF in object)"
;; harness: eval — error-string assertion; compiled error-output is eval-only.
;; oracle: skip — external contrib dep, not on the default oracle classpath;
;; verified manually vs data.json 2.5.1.
(require '[clojure.data.json :as json])
(json/read-str "{\"a\":1")
;; expect-error: JSON error (EOF in object)
