;; print/println strip string quotes and char escapes at EVERY depth, not
;; just the top (fundamentals fix 2026-07-22). print/println are the "human
;; readable" pair — on the JVM they bind *print-readably* nil around the same
;; machinery pr uses, so a string nested in a collection prints WITHOUT quotes.
;; cljgo previously routed print/println through ToString, whose collection
;; branch printed at the default readability, leaving nested quotes ["bad"] —
;; a divergence from JVM print in a very common function.
;;
;; The other half of the contract: `str` KEEPS quotes in collections (it is
;; the readable rendering of the collection), and pr/prn keep them too. Only
;; the human print pair strips. All frozen here so neither side drifts.
;; oracle (clojure 1.12.5, 2026-07-22):
;;   (print-str ["bad" :x])      => "[bad :x]"
;;   (print-str {:k "v"})        => "{:k v}"
;;   (print-str (list "a" "b"))  => "(a b)"
;;   (print-str #{"s"})          => "#{s}"
;;   (print-str \a)              => "a"
;;   (pr-str  ["keep" :x])       => "[\"keep\" :x]"
;;   (str     ["a" :x])          => "[\"a\" :x]"     ; str keeps quotes
;;   (pr-str  \a)                => "\\a"
[(print-str ["bad" :x])
 (print-str {:k "v"})
 (print-str (list "a" "b"))
 (print-str #{"s"})
 (print-str \a)
 (pr-str ["keep" :x])
 (str ["a" :x])
 (pr-str \a)]
;; expect: ["[bad :x]" "{:k v}" "(a b)" "#{s}" "a" "[\"keep\" :x]" "[\"a\" :x]" "\\a"]
