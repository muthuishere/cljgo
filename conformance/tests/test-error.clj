;; clojure.test: an `is` whose body throws unexpectedly (not a thrown? form)
;; is counted as :error, not a crash — every assertion runs under a real
;; (try ... (catch Throwable ...)) like JVM clojure.test's try-expr. Here
;; (= 1 (throw ...)) throws while evaluating the body => one :error.
;; harness: eval — clojure.test is interpreted (ADR 0012); no compiled harness
;; oracle: skip — cljgo's clojure.test bootstrap slice; the :error boundary
;; and summary map shape are standard clojure.test.
(clojure.core/require 'clojure.test)
(clojure.core/refer 'clojure.test)
(deftest t-error
  (is (= 1 (throw (ex-info "kaboom" {})))))
(run-tests 'user)
;; expect: {:test 1, :pass 0, :fail 0, :error 1, :type :summary}
