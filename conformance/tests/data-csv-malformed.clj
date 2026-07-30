;; clojure.data.csv malformed-input errors (ADR 0097). Upstream throws Java
;; EOFException / Exception with StringReader-shaped text; cljgo has no
;; java.io.StringReader, so this port raises its OWN ex-info messages. Per the
;; ADR mandate these are frozen against cljgo's OWN output, NOT the JVM's (there
;; is no Java sink to mirror). Three cases: EOF inside a quoted field, a stray
;; character after a closing quote, and a non-string input to read-csv.
;;
;; oracle: skip — cljgo-authored error text (no JVM oracle for these strings).
(require '[clojure.data.csv :as csv])
[(try (doall (csv/read-csv "\"abc")) :no-throw (catch Throwable e (ex-message e)))
 (try (doall (csv/read-csv "\"a\"x")) :no-throw (catch Throwable e (ex-message e)))
 (try (csv/read-csv 42) :no-throw (catch Throwable e (ex-message e)))]
;; expect: ["clojure.data.csv: unexpected end of input inside a quoted field" "clojure.data.csv: unexpected character after a closing quote: \\x" "clojure.data.csv/read-csv: input must be a string, got: 42"]
