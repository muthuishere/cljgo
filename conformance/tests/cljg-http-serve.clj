;; cljg.http/serve (ADR 0103 wave 1): the raw HTTP server primitive carved out
;; of bri.web.http — ONE handler fn (request-map -> response-map, the same Ring
;; shape bri.web.http speaks at the Go boundary), no routing, no middleware.
;; serve on {:port 0} binds a free loopback port (self-contained, no external
;; network); `addr` reads back the bound host:port; a cljg.net.http GET
;; round-trips status + body + a header; `stop` shuts down gracefully and a
;; second GET then fails (the listener is closed).
;;
;; oracle: skip — cljg.http is a cljgo host namespace (Go net/http server)
;; with no JVM package, so this file cannot run through the `clojure` CLI
;; verbatim. Frozen against cljgo's own output (cljgo-native behavior);
;; REPL-vs-binary parity is enforced by the dual harness.
(require '[cljg.http :as http])
(require '[cljg.net.http :as client])
(let [srv (http/serve {:port 0 :host "127.0.0.1"
                       :handler (fn [req]
                                  {:status 200
                                   :headers {"content-type" "text/plain"}
                                   :body (str "hello " (name (:request-method req))
                                              " " (:uri req))})})
      res (client/get (str "http://" (http/addr srv) "/greet"))
      stopped (http/stop srv)
      down? (try (client/get (str "http://" (http/addr srv) "/greet"))
                 false
                 (catch Throwable _ true))]
  [(:status res)
   (:body res)
   (get (:headers res) "Content-Type")
   (:ok? res)
   stopped
   down?])
;; expect: [200 "hello get /greet" "text/plain" true nil true]
