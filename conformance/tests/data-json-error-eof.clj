;; clojure.data.json read-str on empty input throws with an end-of-file message
;; (ADR 0097). data.json raises java.io.EOFException "JSON error (end-of-file)";
;; cljgo's Go codec raises the same message (matched as a substring).
;; oracle (org.clojure/data.json 2.5.1, verified 2026-07-27):
;;   (json/read-str "") => throws "JSON error (end-of-file)"
;; harness: eval — error-string assertion; the compiled error-output contract
;; is eval-only (same as the other expect-error conformance files).
;; oracle: skip — external contrib dep, not on the default oracle classpath;
;; verified manually vs data.json 2.5.1.
(require '[clojure.data.json :as json])
(json/read-str "")
;; expect-error: JSON error (end-of-file)
