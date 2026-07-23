;; newline + flush (fundamentals batch A1): newline writes "\n" to *out*
;; and returns nil; flush flushes *out* (a no-op on Go's unbuffered
;; stdout, Flush()ed when *out* is a buffered writer) and returns nil.
;; oracle (clojure 1.12.5, 2026-07-23): the exact vector below.
[(with-out-str (prn (newline)))
 (flush)
 (with-out-str (print "a") (newline) (print "b"))]
;; expect: ["\nnil\n" nil "a\nb"]
