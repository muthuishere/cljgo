(ns app.core
  (:require [org.httpkit.server :as hk])
  (:gen-class))

(defn handler [req]
  (case (:uri req)
    "/" {:status 200
         :headers {"Content-Type" "text/plain"}
         :body "hello\n"}
    "/api/hello" {:status 200
                  :headers {"Content-Type" "application/json"}
                  :body "{\"msg\":\"hello from http-kit\"}"}
    {:status 404
     :headers {"Content-Type" "text/plain"}
     :body "not found\n"}))

(defn -main [& _args]
  (let [port (Integer/parseInt (or (System/getenv "PORT") "8080"))]
    (hk/run-server handler {:port port})
    (println "http-kit listening on" port)
    @(promise)))
