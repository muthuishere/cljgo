;; oracle.clj — the s71 differential stress test, JVM half.
;;
;; Reads one java.time pattern per line on stdin, formats the reference
;; instant with each, and writes "pattern<TAB>result" — or "pattern<TAB>!ERR"
;; when java.time itself rejects the pattern.
;;
;; Locale.ENGLISH is FORCED, deliberately. cljgo's translator emits Go layouts,
;; and Go's month/day names are English-only. NOT Locale.ROOT: ROOT collapses MMMM to "Jul" and EEEE to "Fri", so it would
;; have marked correct translations as divergent. Without this line the oracle
;; would silently depend on the machine's default locale and the diff would
;; be a locale artefact rather than a translator defect — which is exactly
;; the trap the ADR documents for callers.
;;
;;   clojure -M oracle.clj < patterns.txt > oracle.tsv

(import '[java.time ZonedDateTime ZoneOffset]
        '[java.time.format DateTimeFormatter]
        '[java.util Locale])

(def ^ZonedDateTime ref
  (ZonedDateTime/of 2026 7 31 9 4 5 123000000 ZoneOffset/UTC))

(doseq [p (line-seq (java.io.BufferedReader. *in*))]
  (println
    (str p "\t"
         (try
           (.format ref (DateTimeFormatter/ofPattern p Locale/ENGLISH))
           (catch Throwable _ "!ERR")))))
