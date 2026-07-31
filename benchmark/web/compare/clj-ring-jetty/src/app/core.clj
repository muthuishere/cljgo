(ns app.core
  (:require [ring.adapter.jetty :as jetty])
  (:gen-class))

(defn handler [req]
  (case (:uri req)
    "/" {:status 200
         :headers {"Content-Type" "text/plain"}
         :body "hello\n"}
    "/api/hello" {:status 200
                  :headers {"Content-Type" "application/json"}
                  :body "{\"msg\":\"hello from ring-jetty\"}"}
    {:status 404
     :headers {"Content-Type" "text/plain"}
     :body "not found\n"}))

(defn -main [& _args]
  (let [port (Integer/parseInt (or (System/getenv "PORT") "8080"))]
    (jetty/run-jetty handler {:port port :join? true})))
