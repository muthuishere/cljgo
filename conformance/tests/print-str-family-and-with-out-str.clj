;; print-str/println-str/prn-str/with-out-str (design/08 batch E, ADR
;; 0022): print/println are human-readable (no quotes on strings); pr/prn
;; are machine-readable (pr-str's formatting); with-out-str binds *out*
;; to a fresh in-memory writer and returns everything printed.
;; oracle (clojure 1.12.5): confirmed interactively via `clojure -e`.
[(print-str "a" 1 :k)
 (println-str "a" 1)
 (prn-str "a" 1)
 (with-out-str (print "a") (print "b"))
 (with-out-str)
 (with-out-str (println "outer")
   (let [inner (with-out-str (println "inner"))]
     (print inner)))]
;; expect: ["a 1 :k" "a 1\n" "\"a\" 1\n" "ab" "" "outer\ninner\n"]
