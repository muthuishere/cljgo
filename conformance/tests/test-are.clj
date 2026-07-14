;; clojure.test (are [argv] expr & rows): the table assertion expands to a
;; do of `is` forms, one per row of (count argv) values. Three rows, all
;; equal => three passes, one deftest.
;; harness: eval — clojure.test is interpreted (ADR 0012); no compiled harness
;; oracle: skip — cljgo's clojure.test bootstrap slice; `are` and the summary
;; map shape are standard clojure.test.
(clojure.core/require 'clojure.test)
(clojure.core/refer 'clojure.test)
(deftest t-are
  (are [x y] (= x y) 1 1 2 2 3 3))
(run-tests 'user)
;; expect: {:test 1, :pass 3, :fail 0, :error 0, :type :summary}
