;; oracle: skip — DEVIATION, not a bug. Real JVM clojure.edn/read-string is
;; actually LENIENT for 3+-part (two-slash) symbols/keywords, same as
;; clojure.core's reader: (clojure.edn/read-string "foo/bar/baz") =>
;; foo/bar/baz (a symbol whose namespace is "foo/bar"), NOT a throw. But
;; the clojure-test-suite (edn_test/read_string.cljc "Invalid Tokens") is
;; written for many dialects at once, and its :default branch (the one
;; cljgo, as a non-JVM/non-CLR dialect, falls into) expects REJECTION —
;; only :clj and :cljr carve out the lenient behavior. cljgo's edn-strict
;; reader mode (pkg/reader/reader.go's ednStrict) matches the suite's
;; :default expectation deliberately, diverging from the real JVM oracle
;; for this one corner. A legitimate 2-part symbol whose NAME is itself a
;; bare "/" (foo//, ns "foo" name "/") must still read fine — it is not a
;; 3-part token.
(require '[clojure.edn :as edn])
[(try (edn/read-string "foo/bar/baz") :no-throw (catch Throwable e :threw))
 (try (edn/read-string ":foo/bar/baz") :no-throw (catch Throwable e :threw))
 (edn/read-string "foo//")]
;; expect: [:threw :threw foo//]
