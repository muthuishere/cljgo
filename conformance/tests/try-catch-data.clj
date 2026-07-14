;; ex-data recovers the data map attached to a caught ex-info.
;; Oracle (clojure 1.12.5):
;; (try (throw (ex-info "boom" {:x 1})) (catch Exception e (ex-data e))) => {:x 1}.
(try
  (throw (ex-info "boom" {:x 1}))
  (catch Exception e (ex-data e)))
;; expect: {:x 1}
