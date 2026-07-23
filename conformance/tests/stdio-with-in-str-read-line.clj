;; with-in-str + read-line (fundamentals batch A1, core gap audit
;; 2026-07-23): with-in-str binds *in* to an in-memory reader
;; (-string-reader, essentials_builtins.go) and read-line reads one line
;; from *in* — the line without its terminator ("\r\n" counts), nil at
;; EOF. This is the deterministic way to test read-line (the harness has
;; no stdin pipe).
;; oracle (clojure 1.12.5, 2026-07-23): the exact vector below.
[(with-in-str "a\nb" [(read-line) (read-line) (read-line)])
 (with-in-str "" (read-line))
 (with-in-str "a\r\nb" [(read-line) (read-line)])
 (with-in-str "one\ntwo" (read-line))]
;; expect: [["a" "b" nil] nil ["a" "b"] "one"]
