;; ADR 0126 made /metrics and CORS secure-by-default and configurable.
;; Both were previously open in every profile. This freezes the postures so
;; the defaults cannot silently drift back open — a security default that
;; nothing asserts is one refactor away from being gone.
;;
;; Env-INDEPENDENT postures only (the suite must not depend on BRI_DEV or
;; APP_METRICS_TOKEN being set or unset in the runner's environment); the
;; token and dev paths are exercised in pkg/bri's Go tests instead.
;;
;; oracle: skip — bri.web.http is a cljgo host namespace with no JVM
;; counterpart, so real Clojure 1.12.5 cannot run this file.

(require '[bri.web.http :as http])

(def rts [["GET /x" (fn [_] {:status 200 :body "ok"})]])

(defn ops-status
  "Status of GET /metrics with the given ops opts, no middleware."
  [opts]
  (:status (http/request rts {:method "GET" :path "/metrics"}
                         (merge {:middleware []} opts))))

(defn acao
  "The access-control-allow-origin header cors emits for a cross-origin GET."
  [opts]
  (get (:headers (http/request rts {:method "GET" :path "/x"
                                    :headers {"origin" "https://evil.example"}}
                               {:middleware [(http/cors opts)] :ops false}))
       "access-control-allow-origin"))

[;; /metrics — explicit opt-in serves it; an explicit guard still wins.
 (ops-status {:metrics-guard :public})
 (ops-status {:metrics-guard {:name :deny
                              :wrap (fn [_] (fn [_] {:status 403 :body "no"}))}})
 ;; CORS — an allowlist echoes only what it allows, never the caller's origin.
 (acao {:origins ["https://app.example"]})
 ;; ... and "*" still works when you ask for it EXPLICITLY. That is the
 ;; difference ADR 0126 draws: permissive on request, never by omission.
 (acao {:origins "*"})]

;; expect: [200 403 "https://app.example" "*"]
