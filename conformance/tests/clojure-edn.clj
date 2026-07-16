;; clojure.edn/read-string (ADR 0022 batch/harness-misc): reads ONE form,
;; never evaluates; empty string reads as nil; the #= eval-reader form throws
;; (cljgo's reader has no #= at all — same observable as EDN's rejection).
;; oracle (clojure 1.12.5):
;;   (clojure.edn/read-string "{:a 1}") => {:a 1}
;;   (clojure.edn/read-string "(1 2 3)") => (1 2 3)
;;   (clojure.edn/read-string "") => nil
;;   (clojure.edn/read-string "foo/bar") => foo/bar (a symbol, unresolved)
;;   (clojure.edn/read-string "#=(+ 1 2)") throws
(require '[clojure.edn :as edn])
[(edn/read-string "{:a 1}")
 (edn/read-string "(1 2 3)")
 (edn/read-string "[1 :k \"s\"]")
 (edn/read-string "")
 (edn/read-string "foo/bar")
 (try (edn/read-string "#=(+ 1 2)") :no-throw (catch Throwable e :threw))]
;; expect: [{:a 1} (1 2 3) [1 :k "s"] nil foo/bar :threw]
