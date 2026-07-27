;; clojure.data.csv/write-csv (ADR 0097): the pure-String writer half of the
;; native port. cljgo has no java.io.Writer, so write-csv takes the data and
;; RETURNS a String (Mandate A: a complete first-class surface, not a degraded
;; one) — byte-identical to what upstream data.csv writes into a
;; java.io.StringWriter. Covers default quoting-only-when-necessary (comma,
;; quote via doubling, embedded newline), a plain field left unquoted, non-string
;; cells via str, a custom :separator, :newline :cr+lf, and a :quote? override.
;;
;; oracle: skip — the pure-String write-csv signature deviates from upstream's
;; (writer, data, …) java.io.Writer sink (cljgo has no Writer), so this file
;; cannot run on the plain `clojure` CLI. Expectations frozen from a JVM
;; org.clojure/data.csv 1.1.0 (Clojure 1.12.5) run through a StringWriter,
;; 2026-07-27:
;;   (let [sw (java.io.StringWriter.)] (apply csv/write-csv sw data opts) (.toString sw))
;;   => ["a,b,c\n1,2,3\n" "\"a,b\",c\n" "\"a\"\"b\"\n" "a,b\r\n" "a;b\n"
;;       "plain,has space\n" "\"line\nbreak\"\n" "1,2,3\n" "\"x\"\n"]
(require '[clojure.data.csv :as csv])
[(csv/write-csv [["a" "b" "c"] ["1" "2" "3"]])
 (csv/write-csv [["a,b" "c"]])
 (csv/write-csv [["a\"b"]])
 (csv/write-csv [["a" "b"]] :newline :cr+lf)
 (csv/write-csv [["a" "b"]] :separator \;)
 (csv/write-csv [["plain" "has space"]])
 (csv/write-csv [["line\nbreak"]])
 (csv/write-csv [[1 2 3]])
 (csv/write-csv [["x"]] :quote? (constantly true))]
;; expect: ["a,b,c\n1,2,3\n" "\"a,b\",c\n" "\"a\"\"b\"\n" "a,b\r\n" "a;b\n" "plain,has space\n" "\"line\nbreak\"\n" "1,2,3\n" "\"x\"\n"]
