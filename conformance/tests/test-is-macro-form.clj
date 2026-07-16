;; clojure.test `is` over a NON-predicate seq form (ADR 0022
;; batch/harness-misc): real clojure.test only apply-splits (pred args...)
;; when the head is a FUNCTION (clojure.test/function?) — a macro or special
;; form head (let, and, when, ...) evaluates as a bare expression instead.
;; Before this fix (is (let [x 1] (= x 1))) tried to (apply let ...), which
;; evaluated the binding vector as data and crashed on unresolved locals.
;; harness: eval — clojure.test is interpreted (ADR 0012); no compiled harness
;; oracle: skip — cljgo's clojure.test bootstrap slice; the summary shape and
;; function?-gating semantics verified against JVM Clojure 1.12.5.
(clojure.core/require 'clojure.test)
(clojure.core/refer 'clojure.test)
(deftest t-macro-forms
  (is (let [x 1] (= x 1)))
  (is (and true 1))
  (is (when true :truthy))
  (is (= 1 1)))
(run-tests 'user)
;; expect: {:test 1, :pass 4, :fail 0, :error 0, :type :summary}
