(defn -main [& args]
  (throw (ex-info "boom at runtime" {:code 42})))
