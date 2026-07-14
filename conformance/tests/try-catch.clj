;; A thrown ex-info is caught by a matching catch clause; ex-message
;; returns the exception's message. Oracle (clojure 1.12.5):
;; (try (throw (ex-info "boom" {:x 1})) (catch Exception e (ex-message e))) => "boom".
(try
  (throw (ex-info "boom" {:x 1}))
  (catch Exception e (ex-message e)))
;; expect: "boom"
