;; S20 criterion 1+2: routes as PLAIN DATA, handlers as VARS, contract
;; Ring-shaped (request map in, response map out). This file is evaluated
;; by the REAL cljgo evaluator; the Go side (main.go) is the adapter the
;; framework would ship — it walks `routes` and mounts each pattern on a
;; net/http ServeMux. Handlers are referenced as #'vars and DEREFED PER
;; REQUEST, so a later (defn hello ...) re-def changes the live server.

(defn hello [req]
  {:status 200
   :body   (str "hello, " (:name (:params req)) " (v1)")})

(defn health [req]
  {:status 200 :body "ok"})

(def routes
  [["GET /hello/{name}" #'hello]
   ["GET /health"       #'health]])
