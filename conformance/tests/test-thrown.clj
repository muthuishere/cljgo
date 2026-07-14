;; clojure.test (is (thrown? Class body)): the assertion passes iff body
;; throws a matching exception, fails otherwise. Here one thrown? throws
;; (pass) and one gets a plain 42 (no throw => fail). run-tests returns the
;; standard clojure.test summary map. Class matching follows the evaluator's
;; try/catch CatchMatches (Exception = any thrown value).
;; harness: eval — clojure.test is interpreted (ADR 0012); no compiled harness
;; oracle: skip — cljgo's clojure.test bootstrap slice; summary map shape is
;; standard clojure.test (run-tests => {:test :pass :fail :error :type}).
(clojure.core/require 'clojure.test)
(clojure.core/refer 'clojure.test)
(deftest t-thrown
  (is (thrown? Exception (throw (ex-info "boom" {}))))
  (is (thrown? Exception 42)))
(run-tests 'user)
;; expect: {:test 1, :pass 1, :fail 1, :error 0, :type :summary}
