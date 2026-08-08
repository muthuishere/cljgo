;; bri.web.http/recover — the error funnel spans BOTH tag keys (ADR 0124).
;; recover dispatches on (:bri/error (ex-data t)) and, failing that, on
;; (:cljg.data.cast/error (ex-data t)), so the mechanism layer (cljg.data.cast)
;; never has to know bri's key. Frozen statuses, in order:
;;
;;   1. a plain ex-info with NO tag           -> 500  (the CONTROL: proves the
;;      funnel is engaged and returning its own status, not a route miss)
;;   2. (db/one! …) with no matching row      -> 404  (:cljg.data.cast/not-found)
;;   3. (db/cast! …) on bad input             -> 422  (:cljg.data.cast/cast)
;;   4. a hand-thrown :bri/error :db/not-found -> 404 (the pre-existing key,
;;      unchanged — http/param! and app code still work exactly as before)
;;
;; Before the fix rows 2 and 3 were 500: cljg.data.cast throws
;; :cljg.data.cast/error, which no row of default-error-map matched.
;;
;; oracle: skip — bri.web.http and cljg.data.cast are cljgo host namespaces
;; (Go net/http + database/sql) with no JVM package, so this file cannot run
;; through the `clojure` CLI verbatim. Frozen against cljgo's own output;
;; REPL-vs-binary parity is enforced by the dual harness.
(require '[bri.web.http :as http])
(require '[cljg.data.cast :as db])
(let [conn   (db/connect {:driver :sqlite :database ":memory:"})
      _      (db/exec! conn "create table notes (id integer primary key, n integer)")
      status (fn [thunk]
               (:status (((:wrap (http/recover)) (fn [_req] (thunk))) {:uri "/t"})))]
  [(status (fn [] (throw (ex-info "boom" {}))))
   (status (fn [] (db/one! conn "select * from notes where id = ?" 7)))
   (status (fn [] (db/cast! {:n "not-a-number"} {:n :int})))
   (status (fn [] (throw (ex-info "gone" {:bri/error :db/not-found}))))])
;; expect: [500 404 422 404]
