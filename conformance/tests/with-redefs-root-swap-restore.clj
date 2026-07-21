;; with-redefs / with-redefs-fn (fundamentals batch 1): temporarily
;; rebind var ROOTS — visible on other threads (unlike binding) — and
;; restore the old roots in finally, throw included. with-redefs-fn is
;; the fn the macro rides on (map of var -> temp value).
;; oracle (clojure 1.12.5): inside => :redef; @(future (f2)) inside =>
;; :redef (root swap, not thread-local); after the form => :orig; after
;; a throwing body => :orig; with-redefs-fn => :redef2; body value
;; passes through => :v.
(defn f2 [] :orig)
(def inside (with-redefs [f2 (fn [] :redef)] (f2)))
(def in-thread (with-redefs [f2 (fn [] :redef)] @(future (f2))))
(def after (f2))
(def after-throw (try (with-redefs [f2 (fn [] :redef)]
                        (throw (ex-info "boom" {})))
                      (catch Exception _e (f2))))
(def via-fn (with-redefs-fn {#'f2 (fn [] :redef2)} (fn [] (f2))))
[inside in-thread after after-throw via-fn
 (with-redefs [f2 (fn [] :redef)] :v)]
;; expect: [:redef :redef :orig :orig :redef2 :v]
