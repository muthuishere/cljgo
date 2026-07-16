;; #inst literals validate their timestamp text the way clojure.instant/
;; validated does — a syntactically matching but calendrically invalid
;; value throws, not just a malformed one (ADR 0022 batch/harness-misc,
;; pkg/reader/tagged.go's NewInst — clojure-test-suite edn_test/
;; read_string.cljc "Tagged Elements Instants"). This is a general reader
;; fix (clojure.core's #inst reads the SAME literal), not edn-specific.
;; oracle (clojure 1.12.5):
;;   (read-string "#inst \"\"") throws
;;   (read-string "#inst \"not-an-inst\"") throws
;;   (read-string "#inst \"2010-02-29T00:00:00.000Z\"") throws (2010 isn't
;;     a leap year, so February only has 28 days)
;;   (read-string "#inst \"2010-01-01T24:00:00.000Z\"") throws (hour must
;;     be 0-23)
;;   (read-string "#inst \"2010-11-12T13:14:15.666-05:00\"") reads fine
(require '[clojure.edn :as edn])
[(try (edn/read-string "#inst \"\"") :no-throw (catch Throwable e :threw))
 (try (edn/read-string "#inst \"not-an-inst\"") :no-throw (catch Throwable e :threw))
 (try (edn/read-string "#inst \"2010-02-29T00:00:00.000Z\"") :no-throw (catch Throwable e :threw))
 (try (edn/read-string "#inst \"2010-01-01T24:00:00.000Z\"") :no-throw (catch Throwable e :threw))
 (.getTime (edn/read-string "#inst \"2010-11-12T13:14:15.666-05:00\""))]
;; expect: [:threw :threw :threw :threw 1289585655666]

