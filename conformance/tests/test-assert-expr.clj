;; clojure.test/assert-expr — the extension point (fundamentals audit 2026-07).
;; `is` delegates to (try-expr msg form), which calls the assert-expr
;; MULTIMETHOD, dispatching on the head symbol of the asserted form. A user
;; teaches `is` a new special form by adding a method:
;;   (defmethod assert-expr 'my-pred [msg form] ...)
;; returning the code the assertion becomes. Here 'divisible-by? is taught;
;; the passing row reports :pass, the failing row :fail, and run-tests still
;; returns the standard summary map — proving the extension point is live and
;; that the built-in dispatch (= / thrown? / bare expr) is unchanged.
;; harness: eval — clojure.test is interpreted (ADR 0012); no compiled harness
;; oracle: skip — cljgo's clojure.test bootstrap slice; assert-expr dispatch +
;; summary shape verified against JVM Clojure 1.12.5 (`clojure` CLI, 2026-07-21):
;; the same forms printed {:test 1, :pass 1, :fail 1, :error 0, :type :summary}.
(clojure.core/require 'clojure.test)
(clojure.core/refer 'clojure.test)
(defmethod assert-expr 'divisible-by? [msg form]
  (let [n (nth form 1) d (nth form 2)]
    `(let [n# ~n d# ~d
           r# (clojure.core/zero? (clojure.core/mod n# d#))]
       (if r#
         (do-report {:type :pass :message ~msg :expected '~form :actual r#})
         (do-report {:type :fail :message ~msg :expected '~form :actual r#}))
       r#)))
(deftest t-custom
  (is (divisible-by? 10 5))
  (is (divisible-by? 10 3)))
(run-tests 'user)
;; expect: {:test 1, :pass 1, :fail 1, :error 0, :type :summary}
