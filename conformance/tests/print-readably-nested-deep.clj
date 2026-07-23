;; print/println strip string quotes AND char escapes at every depth — the
;; deeper shapes print-readably-nested.clj doesn't cover: chars nested in
;; vectors, multi-level nesting (vector-in-map-in-vector), lazy seqs, and the
;; println-str / with-out-str paths. The contract's other half stays frozen
;; too: pr keeps quotes and char escapes inside lazy seqs.
;; *print-readably* is pushed once by the print pair and re-read at every
;; recursive Print call (pkg/lang/strconv.go PrintStringReadably), so one
;; binding covers the whole tree.
;; oracle (clojure 1.12.5, 2026-07-23):
;;   (print-str [\a "b"])                          => "[a b]"
;;   (print-str [["deep" {"k" [\x "y"]}] #{"s"}])  => "[[deep {k [x y]}] #{s}]"
;;   (print-str (map identity ["z" 9]))            => "(z 9)"
;;   (println-str ["ps" \d])                       => "[ps d]\n"
;;   (pr-str (map identity ["z" \a]))              => "(\"z\" \\a)"
;;   (with-out-str (print (list "l" \c)))          => "(l c)"
[(print-str [\a "b"])
 (print-str [["deep" {"k" [\x "y"]}] #{"s"}])
 (print-str (map identity ["z" 9]))
 (println-str ["ps" \d])
 (pr-str (map identity ["z" \a]))
 (with-out-str (print (list "l" \c)))]
;; expect: ["[a b]" "[[deep {k [x y]}] #{s}]" "(z 9)" "[ps d]\n" "(\"z\" \\a)" "(l c)"]
