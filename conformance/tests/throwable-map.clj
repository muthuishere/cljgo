;; Throwable->map (fundamentals batch A2): the JVM-shaped {:cause :via
;; :trace} data view of an exception chain. DEVIATIONS (documented,
;; honest): cljgo has no stack-frame introspection, so :trace is always []
;; and :via entries carry no :at key (the probes below dissoc :at, which
;; is a no-op on cljgo and drops the JVM's frame, so both hosts agree).
;; The typed builtin exception values (ADR 0039 addendum, #99) report
;; their real JVM class name in :via :type — probed with the same
;; errors.Is family `catch` uses, so Throwable->map agrees with catch and
;; instance?; an error OUTSIDE that family falls back to
;; :type java.lang.Exception (deviation: no finer class on the Go host).
;; oracle (clojure 1.12.5, `clojure -M` 2026-07-23):
;;   (def m1 (Throwable->map (ex-info "boom" {:a 1})))
;;   (:cause m1) => "boom"
;;   (map #(dissoc % :at) (:via m1)) => ({:type clojure.lang.ExceptionInfo, :message "boom", :data {:a 1}})
;;   (:data m1) => {:a 1}
;;   (vec (:trace m1)) is a vector — cljgo's is [] (deviation: JVM's has frames, so only emptiness-on-cljgo compatible probes are frozen)
;;   (def m2 (Throwable->map (ex-info "outer" {:o 1} (ex-info "inner" {:i 2}))))
;;   (:cause m2) => "inner"  (the ROOT cause's message)
;;   (map #(dissoc % :at) (:via m2)) => ({:type clojure.lang.ExceptionInfo, :message "outer", :data {:o 1}} {:type clojure.lang.ExceptionInfo, :message "inner", :data {:i 2}})
;;   (:data m2) => {:i 2}  (the root cause's data)
;;   (contains? (Throwable->map (ex-info "x" {})) :data) => true
;; typed-builtin :via :type coverage lives in throwable-map-typed.clj.
(def m1 (Throwable->map (ex-info "boom" {:a 1})))
(def m2 (Throwable->map (ex-info "outer" {:o 1} (ex-info "inner" {:i 2}))))
[(:cause m1)
 (map (fn [v] (dissoc v :at)) (:via m1))
 (:data m1)
 (vector? (:trace m1))
 (:cause m2)
 (map (fn [v] (dissoc v :at)) (:via m2))
 (:data m2)]
;; expect: ["boom" ({:type clojure.lang.ExceptionInfo, :message "boom", :data {:a 1}}) {:a 1} true "inner" ({:type clojure.lang.ExceptionInfo, :message "outer", :data {:o 1}} {:type clojure.lang.ExceptionInfo, :message "inner", :data {:i 2}}) {:i 2}]
