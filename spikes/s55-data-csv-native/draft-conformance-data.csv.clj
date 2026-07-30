;; draft conformance freeze for the cljgo-native clojure.data.csv port
;; (spike s55, ADR 0097 tier-1). DRAFT: these live in the spike dir; the
;; integrator splits each behavior into its own conformance/tests/*.clj
;; file (one form + one ;; expect: per file, house style) and wires the
;; dual (REPL + AOT) harness.
;;
;; oracle: org.clojure/data.csv 1.1.0 on JVM Clojure 1.12.5, captured
;;   2026-07-27 in an isolated dir (a parallel spike had polluted /tmp with
;;   a core.match user.clj — re-run under a clean CLJ_CONFIG):
;;     clojure -Sdeps '{:deps {org.clojure/data.csv {:mvn/version "1.1.0"}}}' -M oracle.clj
;;   read forms: (csv/read-csv <input> <opts>)
;;   write forms captured via a java.io.StringWriter on the JVM; on cljgo
;;   the identical bytes come from (with-out-str (csv/write-csv *out* ...)).
;; Every ;; expect: below is copied verbatim from that oracle run and was
;; reproduced byte-for-byte by the cljgo port (spikes/s55 driver, all 20
;; behaviors IDENTICAL).

(require 'clojure.data.csv)

;; ── READING ──────────────────────────────────────────────────────────

;; 1. basic rows, newline-separated -> lazy seq of vectors
(clojure.data.csv/read-csv "a,b,c\n1,2,3")
;; expect: (["a" "b" "c"] ["1" "2" "3"])

;; 2. quoted cells: separators and newlines inside quotes are literal
(clojure.data.csv/read-csv "\"a,a\",b\n\"c\nc\",d")
;; expect: (["a,a" "b"] ["c\nc" "d"])

;; 3. doubled quote inside a quoted cell -> one literal quote
(clojure.data.csv/read-csv "\"she said \"\"hi\"\"\",x")
;; expect: (["she said \"hi\"" "x"])

;; 4. :separator option
(clojure.data.csv/read-csv "a;b;c" :separator \;)
;; expect: (["a" "b" "c"])

;; 5. :quote option (custom quote char)
(clojure.data.csv/read-csv "'a,a',b" :quote \')
;; expect: (["a,a" "b"])

;; 6. CRLF line endings folded to record boundaries; trailing CRLF dropped
(clojure.data.csv/read-csv "a,b\r\nc,d\r\n")
;; expect: (["a" "b"] ["c" "d"])

;; 7. empty cells preserved (including an all-empty trailing record)
(clojure.data.csv/read-csv "a,,c\n,,")
;; expect: (["a" "" "c"] ["" "" ""])

;; 8. bare CR acts as a record separator (no following LF)
(clojure.data.csv/read-csv "a,b\rc,d")
;; expect: (["a" "b"] ["c" "d"])

;; 9. empty input -> empty seq (the lone [""] record is suppressed)
(clojure.data.csv/read-csv "")
;; expect: ()

;; ── WRITING ──────────────────────────────────────────────────────────
;; (with-out-str (write-csv *out* ...)) == the JVM StringWriter bytes.

;; 10. basic write: LF newline, no unnecessary quoting
(with-out-str (clojure.data.csv/write-csv *out* [["a" "b" "c"] ["1" "2" "3"]]))
;; expect: "a,b,c\n1,2,3\n"

;; 11. auto-quote cells containing sep/quote/CR/LF; embedded quote doubled
(with-out-str (clojure.data.csv/write-csv *out* [["a,a" "b"] ["she said \"hi\""]]))
;; expect: "\"a,a\",b\n\"she said \"\"hi\"\"\"\n"

;; 12. :newline :cr+lf
(with-out-str (clojure.data.csv/write-csv *out* [["a" "b"] ["c" "d"]] :newline :cr+lf))
;; expect: "a,b\r\nc,d\r\n"

;; 13. :quote? predicate forcing quotes on every cell
(with-out-str (clojure.data.csv/write-csv *out* [["a" "b"]] :quote? (constantly true)))
;; expect: "\"a\",\"b\"\n"

;; 14. non-string cells are str-coerced before writing
(with-out-str (clojure.data.csv/write-csv *out* [[1 2 3]]))
;; expect: "1,2,3\n"
