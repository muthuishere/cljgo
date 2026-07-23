;; Stream-based read / read+string + with-in-str (fundamentals batch A2,
;; the reading surface — read-cond option plumbing deliberately untouched,
;; owned elsewhere). cljgo's stream value is the -string-pushback-reader /
;; wrapped-io.Reader coreReadStream (printread_builtins.go), standing in
;; for the JVM's LineNumberingPushbackReader; *in* is the default stream.
;; oracle (clojure 1.12.5, `clojure -M` 2026-07-23):
;;   (with-in-str "[:a :b] rest" (read)) => [:a :b]
;;   (with-in-str "  [1 2] tail" [(read+string *in*) (read+string *in*)])
;;     => [[[1 2] "[1 2]"] [tail "tail"]]   (the string is trimmed)
;;   (with-in-str "1 2 3" [(read) (read) (read)]) => [1 2 3]
;;   (with-in-str "" (read *in* false :my-eof)) => :my-eof
;;   (with-in-str "" (try (read) (catch Exception e :eof-threw))) => :eof-threw
;;     (bare EOF throws; the JVM's message is the wrapped
;;     "java.lang.RuntimeException: EOF while reading" while cljgo's is the
;;     bare "EOF while reading", so only the throw itself is frozen)
;;   (with-in-str "" (read {:eof :done} *in*)) => :done
;;   (with-in-str "42 x" (read+string)) => [42 "42"]
;;   (with-in-str "(+ 1 2)" (read)) => (+ 1 2)
[(with-in-str "[:a :b] rest" (read))
 (with-in-str "  [1 2] tail" [(read+string *in*) (read+string *in*)])
 (with-in-str "1 2 3" [(read) (read) (read)])
 (with-in-str "" (read *in* false :my-eof))
 (with-in-str "" (try (read) (catch Exception e :eof-threw)))
 (with-in-str "" (read {:eof :done} *in*))
 (with-in-str "42 x" (read+string))
 (with-in-str "(+ 1 2)" (read))]
;; expect: [[:a :b] [[[1 2] "[1 2]"] [tail "tail"]] [1 2 3] :my-eof :eof-threw :done [42 "42"] (+ 1 2)]
