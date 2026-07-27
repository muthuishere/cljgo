;; clojure.data.csv/read-csv (ADR 0097): a native cljgo port of
;; org.clojure/data.csv 1.1.0 over a pure String surface. Covers cell/record
;; splitting, quoted fields (embedded separator, doubled-quote escape,
;; embedded newline), the three line endings (LF, CRLF, bare CR), empty fields,
;; a custom :separator, and the trailing-empty-record drop (a final "\n" or ""
;; yields no extra record). read-csv returns a LAZY seq of vectors — realized
;; here with vec.
;;
;; oracle: skip — needs the org.clojure/data.csv dep the plain `clojure` CLI
;; classpath lacks; frozen from a JVM org.clojure/data.csv 1.1.0 (Clojure
;; 1.12.5) run, 2026-07-27:
;;   clojure -Sdeps '{:deps {org.clojure/data.csv {:mvn/version "1.1.0"}}}' -M read_expr.clj
;;   => [[["a" "b" "c"] ["1" "2" "3"]] [["a,b" "c"]] [["a\"b" "c"]]
;;       [["a" "b"]] [] [["a" "b" "c"]] [["a" "b"] ["1" "2"]]
;;       [["a" "" "c"]] [["l1\nl2" "b"]] [["x"] ["y"]]]
(require '[clojure.data.csv :as csv])
[(vec (csv/read-csv "a,b,c\n1,2,3"))
 (vec (csv/read-csv "\"a,b\",c"))
 (vec (csv/read-csv "\"a\"\"b\",c"))
 (vec (csv/read-csv "a,b\n"))
 (vec (csv/read-csv ""))
 (vec (csv/read-csv "a;b;c" :separator \;))
 (vec (csv/read-csv "a,b\r\n1,2"))
 (vec (csv/read-csv "a,,c"))
 (vec (csv/read-csv "\"l1\nl2\",b"))
 (vec (csv/read-csv "x\ry"))]
;; expect: [[["a" "b" "c"] ["1" "2" "3"]] [["a,b" "c"]] [["a\"b" "c"]] [["a" "b"]] [] [["a" "b" "c"]] [["a" "b"] ["1" "2"]] [["a" "" "c"]] [["l1\nl2" "b"]] [["x"] ["y"]]]
