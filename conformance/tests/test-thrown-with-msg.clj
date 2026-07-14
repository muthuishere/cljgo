;; clojure.test (is (thrown-with-msg? Class #"re" body)): passes iff body
;; throws a matching class AND the exception message matches the regex.
;; First row: message "boom!" matches #"bo+m" (pass). Second row: message
;; "boom!" does not match #"xyz" (fail). Real regex via the -re-find? host
;; seam (cljgo has no clojure.core/re-find yet).
;; harness: eval — clojure.test is interpreted (ADR 0012); no compiled harness
;; oracle: skip — cljgo's clojure.test bootstrap slice; thrown-with-msg? and
;; the summary map shape are standard clojure.test.
(clojure.core/require 'clojure.test)
(clojure.core/refer 'clojure.test)
(deftest t-msg
  (is (thrown-with-msg? Exception #"bo+m" (throw (ex-info "boom!" {}))))
  (is (thrown-with-msg? Exception #"xyz" (throw (ex-info "boom!" {})))))
(run-tests 'user)
;; expect: {:test 1, :pass 1, :fail 1, :error 0, :type :summary}
