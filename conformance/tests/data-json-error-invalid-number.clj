;; clojure.data.json read-str on a malformed number (a JSON-forbidden leading
;; zero) throws an invalid-number-literal message (ADR 0097).
;; oracle (org.clojure/data.json 2.5.1, verified 2026-07-27):
;;   (json/read-str "01") => throws "JSON error (invalid number literal)"
;; harness: eval — error-string assertion; compiled error-output is eval-only.
;; oracle: skip — external contrib dep, not on the default oracle classpath;
;; verified manually vs data.json 2.5.1.
(require '[clojure.data.json :as json])
(json/read-str "01")
;; expect-error: JSON error (invalid number literal)
