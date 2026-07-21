;; vary-meta (fundamentals audit 2026-07): same value, metadata replaced by
;; (apply f (meta obj) args).
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (vary-meta (with-meta [1] {:a 1}) assoc :b 2) => [1]
;;   (meta (vary-meta (with-meta [1] {:a 1}) assoc :b 2)) => {:a 1, :b 2}
;;   (meta (vary-meta [1] assoc :b 2)) => {:b 2}
[(vary-meta (with-meta [1] {:a 1}) assoc :b 2)
 (meta (vary-meta (with-meta [1] {:a 1}) assoc :b 2))
 (meta (vary-meta [1] assoc :b 2))]
;; expect: [[1] {:a 1, :b 2} {:b 2}]
