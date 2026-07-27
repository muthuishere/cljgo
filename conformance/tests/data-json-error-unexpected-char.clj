;; clojure.data.json read-str on a bad token throws an unexpected-character
;; message naming the offending char (ADR 0097).
;; oracle (org.clojure/data.json 2.5.1, verified 2026-07-27):
;;   (json/read-str "xyz") => throws "JSON error (unexpected character): x"
;; harness: eval — error-string assertion; compiled error-output is eval-only.
;; oracle: skip — external contrib dep, not on the default oracle classpath;
;; verified manually vs data.json 2.5.1.
(require '[clojure.data.json :as json])
(json/read-str "xyz")
;; expect-error: JSON error (unexpected character): x
