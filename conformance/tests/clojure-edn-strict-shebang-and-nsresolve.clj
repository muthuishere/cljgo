;; clojure.edn/read-string is a RESTRICTED reader (ADR 0022 batch/
;; harness-misc, pkg/reader/reader.go's ednStrict — clojure-test-suite
;; edn_test/read_string.cljc "Invalid Tokens"): unlike clojure.core's
;; reader, it has no `#!` shebang comment macro and no current-namespace
;; context for `::kw` auto-resolution, so both throw. oracle (clojure
;; 1.12.5):
;;   (clojure.edn/read-string "#!shebang") throws "No dispatch macro for: !"
;;   (clojure.edn/read-string "#!shebang\r\n1") throws (same reason)
;;   (clojure.edn/read-string "::foo") throws "Invalid token: ::foo"
;; ednStrict is set ONLY by clojure.edn/read-string (precedence principle —
;; edn only tightens, never changes clojure.core's reader — pkg/reader
;; unit tests TestShebangComment / auto-resolved-keyword cover the
;; unaffected clojure.core side; there is no public clojure.core/read-string
;; yet to exercise it from Clojure source).
(require '[clojure.edn :as edn])
[(try (edn/read-string "#!shebang") :no-throw (catch Throwable e :threw))
 (try (edn/read-string "#!shebang\r\n1") :no-throw (catch Throwable e :threw))
 (try (edn/read-string "::foo") :no-throw (catch Throwable e :threw))]
;; expect: [:threw :threw :threw]
