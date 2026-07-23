;; Throwable->map over the TYPED builtin exception values (ADR 0039
;; addendum, #99): :via :type carries the real JVM class name, probed with
;; the same errors.Is family `catch` uses (corelib throwableTypeSym), so
;; Throwable->map agrees with catch and instance? about the same value.
;; Probes dissoc :at (no stack-frame introspection on cljgo — a no-op
;; here, drops the JVM's frame, both hosts agree). The nth probe also
;; dissocs :message — the JVM's IndexOutOfBounds message is null while
;; cljgo's names the index (documented deviation).
;; oracle (clojure 1.12.5, `clojure -M` 2026-07-23):
;;   (let [m (Throwable->map (try (/ 1 0) (catch Throwable t t)))]
;;     [(:cause m) (map #(dissoc % :at) (:via m))])
;;   => ["Divide by zero" ({:type java.lang.ArithmeticException, :message "Divide by zero"})]
;;   (map #(dissoc % :at :message) (:via (Throwable->map (try (nth [1] 5) (catch Throwable t t)))))
;;   => ({:type java.lang.IndexOutOfBoundsException})
(def m1 (Throwable->map (try (/ 1 0) (catch Throwable t t))))
(def m2 (Throwable->map (try (nth [1] 5) (catch Throwable t t))))
[(:cause m1)
 (map (fn [v] (dissoc v :at)) (:via m1))
 (map (fn [v] (dissoc v :at :message)) (:via m2))]
;; expect: ["Divide by zero" ({:type java.lang.ArithmeticException, :message "Divide by zero"}) ({:type java.lang.IndexOutOfBoundsException})]
